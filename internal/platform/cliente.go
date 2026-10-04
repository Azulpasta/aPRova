package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const (
	versaoDaAPI             = "2022-11-28"
	tamanhoMaximoDaResposta = 1 << 20
)

var (
	// ErrNaoEncontrado indica recurso inexistente ou invisível para a instalação.
	ErrNaoEncontrado = errors.New("github: recurso não encontrado")

	// ErrRespostaInesperada indica resposta de erro que não é indisponibilidade
	// nem recurso ausente.
	ErrRespostaInesperada = errors.New("github: resposta inesperada")

	// ErrPermissaoDesconhecida indica papel devolvido pelo GitHub que o aPRova
	// não reconhece.
	ErrPermissaoDesconhecida = errors.New("github: papel de colaborador desconhecido")
)

// FornecedorDeToken entrega o token de uma instalação do GitHub App.
type FornecedorDeToken interface {
	TokenParaInstalacao(ctx context.Context, instalacaoID int64) (string, error)
}

// OpcoesCliente reúne o que o cliente precisa para chamar a API do GitHub.
type OpcoesCliente struct {
	Tokens      FornecedorDeToken
	URLBase     string
	ClienteHTTP *http.Client
}

// Cliente chama a API do GitHub em nome de uma instalação do app. Nenhuma
// chamada é repetida automaticamente.
type Cliente struct {
	tokens      FornecedorDeToken
	urlBase     string
	clienteHTTP *http.Client
}

type chamada struct {
	instalacaoID int64
	metodo       string
	caminho      string
	corpo        any
	resposta     any
}

// NovoCliente constrói o cliente a partir das opções informadas.
func NovoCliente(opcoes OpcoesCliente) *Cliente {
	return &Cliente{
		tokens:      opcoes.Tokens,
		urlBase:     valorOuPadrao(opcoes.URLBase, urlBasePadrao),
		clienteHTTP: clienteOuPadrao(opcoes.ClienteHTTP),
	}
}

// PermissaoNoRepositorio consulta o papel do login no repositório pelo
// endpoint de permissão de colaborador. Qualquer resposta que não seja um
// papel reconhecido devolve erro.
func (c *Cliente) PermissaoNoRepositorio(ctx context.Context, installationID int64, dono, repositorio, login string) (Permissao, error) {
	var resposta struct {
		Permissao string `json:"permission"`
		Papel     string `json:"role_name"`
	}

	err := c.chamar(ctx, chamada{
		instalacaoID: installationID,
		metodo:       http.MethodGet,
		caminho:      caminhoDoRepositorio(dono, repositorio) + "/collaborators/" + url.PathEscape(login) + "/permission",
		resposta:     &resposta,
	})
	if err != nil {
		return "", err
	}

	return interpretarPermissao(resposta.Papel, resposta.Permissao)
}

// AutorDoPullRequest devolve o login de quem abriu o pull request.
func (c *Cliente) AutorDoPullRequest(ctx context.Context, installationID int64, dono, repositorio string, numero int) (string, error) {
	var resposta struct {
		Usuario *struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	err := c.chamar(ctx, chamada{
		instalacaoID: installationID,
		metodo:       http.MethodGet,
		caminho:      caminhoDoPullRequest(dono, repositorio, numero),
		resposta:     &resposta,
	})
	if err != nil {
		return "", err
	}

	if resposta.Usuario == nil || resposta.Usuario.Login == "" {
		return "", fmt.Errorf("%w: pull request sem autor", ErrRespostaInesperada)
	}
	return resposta.Usuario.Login, nil
}

// EnviarReview publica a review no pull request.
func (c *Cliente) EnviarReview(ctx context.Context, installationID int64, dono, repositorio string, numero int, review Review) error {
	return c.chamar(ctx, chamada{
		instalacaoID: installationID,
		metodo:       http.MethodPost,
		caminho:      caminhoDoPullRequest(dono, repositorio, numero) + "/reviews",
		corpo:        map[string]string{"event": string(review.Evento), "body": review.Corpo},
	})
}

func (c *Cliente) chamar(ctx context.Context, pedido chamada) error {
	token, err := c.tokens.TokenParaInstalacao(ctx, pedido.instalacaoID)
	if err != nil {
		return err
	}

	requisicao, err := c.montarRequisicao(ctx, pedido, token)
	if err != nil {
		return err
	}

	resposta, err := c.clienteHTTP.Do(requisicao)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrGitHubIndisponivel, err)
	}
	defer func() { _ = resposta.Body.Close() }()

	registrarSaldoDeRequisicoes(resposta, pedido.instalacaoID)

	if err := classificarRespostaDaAPI(resposta.StatusCode, pedido); err != nil {
		return err
	}

	if pedido.resposta == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resposta.Body, tamanhoMaximoDaResposta)).Decode(pedido.resposta); err != nil {
		return fmt.Errorf("%w: corpo ilegível em %s %s: %w", ErrRespostaInesperada, pedido.metodo, pedido.caminho, err)
	}
	return nil
}

func (c *Cliente) montarRequisicao(ctx context.Context, pedido chamada, token string) (*http.Request, error) {
	var corpo io.Reader = http.NoBody
	if pedido.corpo != nil {
		serializado, err := json.Marshal(pedido.corpo)
		if err != nil {
			return nil, fmt.Errorf("serializar corpo de %s: %w", pedido.caminho, err)
		}
		corpo = bytes.NewReader(serializado)
	}

	requisicao, err := http.NewRequestWithContext(ctx, pedido.metodo, c.urlBase+pedido.caminho, corpo)
	if err != nil {
		return nil, fmt.Errorf("montar requisição para %s: %w", pedido.caminho, err)
	}

	requisicao.Header.Set("Authorization", "Bearer "+token)
	requisicao.Header.Set("Accept", "application/vnd.github+json")
	requisicao.Header.Set("X-GitHub-Api-Version", versaoDaAPI)
	if pedido.corpo != nil {
		requisicao.Header.Set("Content-Type", "application/json")
	}

	return requisicao, nil
}

func classificarRespostaDaAPI(status int, pedido chamada) error {
	switch {
	case status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s %s", ErrNaoEncontrado, pedido.metodo, pedido.caminho)
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %s %s respondeu %d", ErrGitHubIndisponivel, pedido.metodo, pedido.caminho, status)
	default:
		return fmt.Errorf("%w: %s %s respondeu %d", ErrRespostaInesperada, pedido.metodo, pedido.caminho, status)
	}
}

func caminhoDoRepositorio(dono, repositorio string) string {
	return "/repos/" + url.PathEscape(dono) + "/" + url.PathEscape(repositorio)
}

func caminhoDoPullRequest(dono, repositorio string, numero int) string {
	return caminhoDoRepositorio(dono, repositorio) + "/pulls/" + strconv.Itoa(numero)
}
