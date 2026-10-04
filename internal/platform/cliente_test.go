package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const tokenDeInstalacaoDeTeste = "ghs_token-de-teste"

type fornecedorDeTokenFalso struct {
	pedidos []int64
	erro    error
}

func (f *fornecedorDeTokenFalso) TokenParaInstalacao(_ context.Context, instalacaoID int64) (string, error) {
	f.pedidos = append(f.pedidos, instalacaoID)
	return tokenDeInstalacaoDeTeste, f.erro
}

type requisicaoRecebida struct {
	metodo        string
	caminho       string
	autorizacao   string
	versaoDaAPI   string
	tipoDoCorpo   string
	corpo         map[string]any
	quantasVieram int
}

func servidorDaAPIQueResponde(t *testing.T, status int, corpo string) (*httptest.Server, *requisicaoRecebida) {
	t.Helper()

	recebida := &requisicaoRecebida{}
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recebida.quantasVieram++
		recebida.metodo = r.Method
		recebida.caminho = r.URL.EscapedPath()
		recebida.autorizacao = r.Header.Get("Authorization")
		recebida.versaoDaAPI = r.Header.Get("X-GitHub-Api-Version")
		recebida.tipoDoCorpo = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&recebida.corpo)

		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}))
	t.Cleanup(servidor.Close)

	return servidor, recebida
}

func clienteContra(servidor *httptest.Server, tokens *fornecedorDeTokenFalso) *Cliente {
	return NovoCliente(OpcoesCliente{Tokens: tokens, URLBase: servidor.URL, ClienteHTTP: servidor.Client()})
}

func TestPermissaoNoRepositorioConsultaOEndpointDeColaboradorComOTokenDaInstalacao(t *testing.T) {
	servidor, recebida := servidorDaAPIQueResponde(t, http.StatusOK, `{"permission":"write","role_name":"write"}`)
	tokens := &fornecedorDeTokenFalso{}

	permissao, err := clienteContra(servidor, tokens).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

	if err != nil {
		t.Fatalf("consulta não deveria falhar: %v", err)
	}
	if permissao != PermissaoWrite {
		t.Errorf("permissão = %q, esperava write", permissao)
	}
	if recebida.metodo != http.MethodGet || recebida.caminho != "/repos/Azulpasta/aPRova/collaborators/erin/permission" {
		t.Errorf("chamada errada: %s %s", recebida.metodo, recebida.caminho)
	}
	if recebida.autorizacao != "Bearer "+tokenDeInstalacaoDeTeste || recebida.versaoDaAPI != "2022-11-28" {
		t.Errorf("cabeçalhos errados: autorização %q, versão %q", recebida.autorizacao, recebida.versaoDaAPI)
	}
	if len(tokens.pedidos) != 1 || tokens.pedidos[0] != 7001 {
		t.Errorf("o token precisa ser o da instalação 7001, pedidos: %v", tokens.pedidos)
	}
}

func TestPermissaoNoRepositorioUsaOPapelDetalhado(t *testing.T) {
	casos := []struct {
		resposta string
		esperada Permissao
	}{
		{`{"permission":"admin","role_name":"admin"}`, PermissaoAdmin},
		{`{"permission":"write","role_name":"maintain"}`, PermissaoMaintain},
		{`{"permission":"write","role_name":"write"}`, PermissaoWrite},
		{`{"permission":"read","role_name":"triage"}`, PermissaoTriage},
		{`{"permission":"read","role_name":"read"}`, PermissaoRead},
		{`{"permission":"none","role_name":"none"}`, PermissaoNenhuma},
		{`{"permission":"write","role_name":"revisor-de-seguranca"}`, PermissaoWrite},
		{`{"permission":"read"}`, PermissaoRead},
	}

	for _, caso := range casos {
		t.Run(caso.resposta, func(t *testing.T) {
			servidor, _ := servidorDaAPIQueResponde(t, http.StatusOK, caso.resposta)

			permissao, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

			if err != nil || permissao != caso.esperada {
				t.Errorf("obtive %q e %v, esperava %q", permissao, err, caso.esperada)
			}
		})
	}
}

func TestPermissaoNoRepositorioFalhaComPapelQueNaoReconhece(t *testing.T) {
	servidor, _ := servidorDaAPIQueResponde(t, http.StatusOK, `{"permission":"superuser","role_name":"superuser"}`)

	_, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

	if !errors.Is(err, ErrPermissaoDesconhecida) {
		t.Errorf("papel desconhecido não pode virar permissão, obtive %v", err)
	}
}

func TestPermissaoNoRepositorioDevolveErroEmRespostaQueNaoEhSucesso(t *testing.T) {
	casos := []struct {
		status   int
		esperado error
	}{
		{http.StatusNotFound, ErrNaoEncontrado},
		{http.StatusInternalServerError, ErrGitHubIndisponivel},
		{http.StatusBadGateway, ErrGitHubIndisponivel},
		{http.StatusForbidden, ErrRespostaInesperada},
		{http.StatusUnauthorized, ErrRespostaInesperada},
	}

	for _, caso := range casos {
		t.Run(http.StatusText(caso.status), func(t *testing.T) {
			servidor, _ := servidorDaAPIQueResponde(t, caso.status, `{"permission":"admin","role_name":"admin"}`)

			permissao, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

			if !errors.Is(err, caso.esperado) {
				t.Errorf("esperava %v, obtive %v", caso.esperado, err)
			}
			if permissao != "" {
				t.Errorf("com erro, nenhuma permissão pode ser devolvida, obtive %q", permissao)
			}
		})
	}
}

func TestPermissaoNoRepositorioFalhaComCorpoQueNaoEhJSON(t *testing.T) {
	servidor, _ := servidorDaAPIQueResponde(t, http.StatusOK, `<html>manutenção</html>`)

	_, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

	if err == nil {
		t.Error("corpo ilegível não pode ser tratado como permissão")
	}
}

func TestPermissaoNoRepositorioNaoChamaOGitHubSemToken(t *testing.T) {
	servidor, recebida := servidorDaAPIQueResponde(t, http.StatusOK, `{"permission":"admin","role_name":"admin"}`)
	tokens := &fornecedorDeTokenFalso{erro: ErrAppNaoInstalado}

	_, err := clienteContra(servidor, tokens).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "erin")

	if !errors.Is(err, ErrAppNaoInstalado) {
		t.Errorf("a falha do token precisa chegar a quem chamou, obtive %v", err)
	}
	if recebida.quantasVieram != 0 {
		t.Error("sem token, nenhuma chamada pode sair")
	}
}

func TestPermissaoNoRepositorioEscapaOLoginNoCaminho(t *testing.T) {
	servidor, recebida := servidorDaAPIQueResponde(t, http.StatusOK, `{"permission":"read","role_name":"read"}`)

	_, _ = clienteContra(servidor, &fornecedorDeTokenFalso{}).PermissaoNoRepositorio(t.Context(), 7001, "Azulpasta", "aPRova", "../../../user")

	if recebida.caminho != "/repos/Azulpasta/aPRova/collaborators/..%2F..%2F..%2Fuser/permission" {
		t.Errorf("o login não pode mudar o endpoint chamado, caminho: %s", recebida.caminho)
	}
}

func TestAutorDoPullRequestDevolveOLoginDeQuemAbriu(t *testing.T) {
	servidor, recebida := servidorDaAPIQueResponde(t, http.StatusOK, `{"number":42,"user":{"login":"carol"}}`)

	autor, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).AutorDoPullRequest(t.Context(), 7001, "Azulpasta", "aPRova", 42)

	if err != nil || autor != "carol" {
		t.Fatalf("esperava carol, obtive %q e %v", autor, err)
	}
	if recebida.metodo != http.MethodGet || recebida.caminho != "/repos/Azulpasta/aPRova/pulls/42" {
		t.Errorf("chamada errada: %s %s", recebida.metodo, recebida.caminho)
	}
}

func TestAutorDoPullRequestFalhaSemLogin(t *testing.T) {
	servidor, _ := servidorDaAPIQueResponde(t, http.StatusOK, `{"number":42,"user":null}`)

	_, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).AutorDoPullRequest(t.Context(), 7001, "Azulpasta", "aPRova", 42)

	if err == nil {
		t.Error("PR sem autor identificável não pode passar pela regra de autoria")
	}
}

func TestAutorDoPullRequestDevolveErroQuandoOPRNaoExiste(t *testing.T) {
	servidor, _ := servidorDaAPIQueResponde(t, http.StatusNotFound, `{"message":"Not Found"}`)

	_, err := clienteContra(servidor, &fornecedorDeTokenFalso{}).AutorDoPullRequest(t.Context(), 7001, "Azulpasta", "aPRova", 42)

	if !errors.Is(err, ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func TestEnviarReviewPublicaOEventoEOCorpo(t *testing.T) {
	servidor, recebida := servidorDaAPIQueResponde(t, http.StatusOK, `{"id":1}`)

	err := clienteContra(servidor, &fornecedorDeTokenFalso{}).EnviarReview(t.Context(), 7001, "Azulpasta", "aPRova", 42, Review{
		Evento: EventoAprovar,
		Corpo:  "Aprovado por @erin via Slack",
	})

	if err != nil {
		t.Fatalf("envio não deveria falhar: %v", err)
	}
	if recebida.metodo != http.MethodPost || recebida.caminho != "/repos/Azulpasta/aPRova/pulls/42/reviews" {
		t.Errorf("chamada errada: %s %s", recebida.metodo, recebida.caminho)
	}
	if recebida.corpo["event"] != "APPROVE" || recebida.corpo["body"] != "Aprovado por @erin via Slack" {
		t.Errorf("corpo enviado errado: %v", recebida.corpo)
	}
	if recebida.tipoDoCorpo != "application/json" || recebida.autorizacao != "Bearer "+tokenDeInstalacaoDeTeste {
		t.Errorf("cabeçalhos errados: tipo %q, autorização %q", recebida.tipoDoCorpo, recebida.autorizacao)
	}
}

func TestEnviarReviewDevolveErroQuandoOGitHubRecusa(t *testing.T) {
	casos := []struct {
		status   int
		esperado error
	}{
		{http.StatusUnprocessableEntity, ErrRespostaInesperada},
		{http.StatusInternalServerError, ErrGitHubIndisponivel},
	}

	for _, caso := range casos {
		t.Run(http.StatusText(caso.status), func(t *testing.T) {
			servidor, recebida := servidorDaAPIQueResponde(t, caso.status, `{"message":"falhou"}`)

			err := clienteContra(servidor, &fornecedorDeTokenFalso{}).EnviarReview(t.Context(), 7001, "Azulpasta", "aPRova", 42, Review{Evento: EventoAprovar})

			if !errors.Is(err, caso.esperado) {
				t.Errorf("esperava %v, obtive %v", caso.esperado, err)
			}
			if recebida.quantasVieram != 1 {
				t.Errorf("escrita não é repetida automaticamente, vieram %d chamadas", recebida.quantasVieram)
			}
		})
	}
}
