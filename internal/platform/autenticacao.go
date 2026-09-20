package platform

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	urlBasePadrao       = "https://api.github.com"
	margemDeRenovacao   = 5 * time.Minute
	validadeDoJWT       = 9 * time.Minute
	toleranciaDeRelogio = 60 * time.Second
	tempoLimiteHTTP     = 10 * time.Second

	tentativasParaTransitorio = 3
	tentativasParaCredencial  = 2
	esperaBaseEntreTentativas = 500 * time.Millisecond
)

var (
	// ErrConfiguracaoInvalida indica credencial do app inutilizável, que não se
	// resolve repetindo a chamada.
	ErrConfiguracaoInvalida = errors.New("github: configuração do app inválida")

	// ErrAppNaoInstalado indica que o app não está instalado na instalação
	// pedida. Não é transitório e repetir não adianta.
	ErrAppNaoInstalado = errors.New("github: app não instalado na instalação informada")

	// ErrCredencialRejeitada indica JWT recusado, em geral por relógio
	// dessincronizado ou token já expirado. Uma nova tentativa costuma resolver.
	ErrCredencialRejeitada = errors.New("github: credencial do app rejeitada")

	// ErrGitHubIndisponivel indica falha transitória do GitHub, que merece nova
	// tentativa com espera crescente.
	ErrGitHubIndisponivel = errors.New("github: serviço indisponível")
)

// OpcoesAutenticador reúne o que o autenticador precisa para falar com o GitHub.
type OpcoesAutenticador struct {
	AppID           string
	ChavePrivadaPEM []byte
	URLBase         string
	ClienteHTTP     *http.Client
	Agora           func() time.Time
	Espera          func(time.Duration)
}

type tokenDeInstalacao struct {
	valor    string
	expiraEm time.Time
}

// Autenticador obtém e mantém em memória os tokens de instalação do GitHub App.
type Autenticador struct {
	appID        string
	chavePrivada *rsa.PrivateKey
	urlBase      string
	clienteHTTP  *http.Client
	agora        func() time.Time
	espera       func(time.Duration)

	renovacoes singleflight.Group

	mutex  sync.Mutex
	tokens map[int64]tokenDeInstalacao
}

// NovoAutenticador interpreta a chave privada e constrói o autenticador. Uma
// chave malformada devolve erro aqui, na inicialização, e não na primeira
// chamada à API.
func NovoAutenticador(opcoes OpcoesAutenticador) (*Autenticador, error) {
	if strings.TrimSpace(opcoes.AppID) == "" {
		return nil, fmt.Errorf("%w: app id não informado", ErrConfiguracaoInvalida)
	}

	chavePrivada, err := interpretarChavePrivada(opcoes.ChavePrivadaPEM)
	if err != nil {
		return nil, err
	}

	return &Autenticador{
		appID:        opcoes.AppID,
		chavePrivada: chavePrivada,
		urlBase:      valorOuPadrao(opcoes.URLBase, urlBasePadrao),
		clienteHTTP:  clienteOuPadrao(opcoes.ClienteHTTP),
		agora:        relogioOuPadrao(opcoes.Agora),
		espera:       esperaOuPadrao(opcoes.Espera),
		tokens:       map[int64]tokenDeInstalacao{},
	}, nil
}

// TokenParaInstalacao devolve um token de instalação válido, renovando-o quando
// estiver perto de expirar. Quem chama não precisa conhecer o JWT nem o cache.
func (a *Autenticador) TokenParaInstalacao(ctx context.Context, instalacaoID int64) (string, error) {
	if token, aproveitavel := a.tokenEmCache(instalacaoID); aproveitavel {
		return token, nil
	}

	renovado, err, _ := a.renovacoes.Do(strconv.FormatInt(instalacaoID, 10), func() (any, error) {
		if token, aproveitavel := a.tokenEmCache(instalacaoID); aproveitavel {
			return token, nil
		}

		token, err := a.solicitarTokenComRetentativa(ctx, instalacaoID)
		if err != nil {
			return "", err
		}

		a.mutex.Lock()
		a.tokens[instalacaoID] = token
		a.mutex.Unlock()

		return token.valor, nil
	})
	if err != nil {
		return "", err
	}

	return renovado.(string), nil
}

func (a *Autenticador) solicitarTokenComRetentativa(ctx context.Context, instalacaoID int64) (tokenDeInstalacao, error) {
	tentativa := 1

	for {
		token, err := a.solicitarToken(ctx, instalacaoID)
		if err == nil {
			return token, nil
		}

		if tentativa >= limiteDeTentativas(err) {
			return tokenDeInstalacao{}, err
		}

		slog.Warn("nova tentativa de obter token de instalação",
			"instalacao_id", instalacaoID, "tentativa", tentativa, "erro", err)

		a.espera(esperaBaseEntreTentativas << (tentativa - 1))
		tentativa++
	}
}

func (a *Autenticador) tokenEmCache(instalacaoID int64) (string, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	token, existe := a.tokens[instalacaoID]
	if !existe {
		return "", false
	}

	if a.agora().Add(margemDeRenovacao).After(token.expiraEm) {
		return "", false
	}

	return token.valor, true
}

func (a *Autenticador) solicitarToken(ctx context.Context, instalacaoID int64) (tokenDeInstalacao, error) {
	autorizacao, err := a.gerarJWT()
	if err != nil {
		return tokenDeInstalacao{}, err
	}

	endereco := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.urlBase, instalacaoID)
	requisicao, err := http.NewRequestWithContext(ctx, http.MethodPost, endereco, nil)
	if err != nil {
		return tokenDeInstalacao{}, fmt.Errorf("montar requisição de token: %w", err)
	}
	requisicao.Header.Set("Authorization", "Bearer "+autorizacao)
	requisicao.Header.Set("Accept", "application/vnd.github+json")

	resposta, err := a.clienteHTTP.Do(requisicao)
	if err != nil {
		return tokenDeInstalacao{}, fmt.Errorf("chamar github: %w", err)
	}
	defer func() { _ = resposta.Body.Close() }()

	registrarSaldoDeRequisicoes(resposta, instalacaoID)

	if err := classificarStatus(resposta.StatusCode); err != nil {
		return tokenDeInstalacao{}, err
	}

	var corpo struct {
		Token    string    `json:"token"`
		ExpiraEm time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resposta.Body).Decode(&corpo); err != nil {
		return tokenDeInstalacao{}, fmt.Errorf("interpretar resposta de token: %w", err)
	}

	return tokenDeInstalacao{valor: corpo.Token, expiraEm: corpo.ExpiraEm}, nil
}

func (a *Autenticador) gerarJWT() (string, error) {
	agora := a.agora()

	cabecalho, err := codificarEmBase64URL(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}

	reivindicacoes, err := codificarEmBase64URL(map[string]any{
		"iss": a.appID,
		"iat": agora.Add(-toleranciaDeRelogio).Unix(),
		"exp": agora.Add(validadeDoJWT).Unix(),
	})
	if err != nil {
		return "", err
	}

	conteudo := cabecalho + "." + reivindicacoes
	resumo := sha256.Sum256([]byte(conteudo))

	assinatura, err := rsa.SignPKCS1v15(rand.Reader, a.chavePrivada, crypto.SHA256, resumo[:])
	if err != nil {
		return "", fmt.Errorf("assinar jwt: %w", err)
	}

	return conteudo + "." + base64.RawURLEncoding.EncodeToString(assinatura), nil
}

func classificarStatus(status int) error {
	switch {
	case status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusNotFound:
		return fmt.Errorf("%w (status %d)", ErrAppNaoInstalado, status)
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%w (status %d)", ErrCredencialRejeitada, status)
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("%w (status %d)", ErrGitHubIndisponivel, status)
	default:
		return fmt.Errorf("github respondeu %d ao trocar o jwt", status)
	}
}

func limiteDeTentativas(err error) int {
	switch {
	case errors.Is(err, ErrGitHubIndisponivel):
		return tentativasParaTransitorio
	case errors.Is(err, ErrCredencialRejeitada):
		return tentativasParaCredencial
	default:
		return 1
	}
}

func registrarSaldoDeRequisicoes(resposta *http.Response, instalacaoID int64) {
	restantes := resposta.Header.Get("X-RateLimit-Remaining")
	if restantes == "" {
		return
	}

	slog.Info("saldo de requisições do github",
		"instalacao_id", instalacaoID,
		"restantes", restantes,
		"limite", resposta.Header.Get("X-RateLimit-Limit"),
		"reinicia_em", resposta.Header.Get("X-RateLimit-Reset"))
}

func codificarEmBase64URL(valor any) (string, error) {
	serializado, err := json.Marshal(valor)
	if err != nil {
		return "", fmt.Errorf("serializar parte do jwt: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(serializado), nil
}

func interpretarChavePrivada(pemBruto []byte) (*rsa.PrivateKey, error) {
	bloco, _ := pem.Decode(pemBruto)
	if bloco == nil {
		return nil, fmt.Errorf("%w: a chave privada do app não está em PEM válido", ErrConfiguracaoInvalida)
	}

	if chave, err := x509.ParsePKCS1PrivateKey(bloco.Bytes); err == nil {
		return chave, nil
	}

	generica, err := x509.ParsePKCS8PrivateKey(bloco.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: a chave privada do app não pôde ser interpretada", ErrConfiguracaoInvalida)
	}

	chave, ehRSA := generica.(*rsa.PrivateKey)
	if !ehRSA {
		return nil, fmt.Errorf("%w: a chave privada do app precisa ser RSA", ErrConfiguracaoInvalida)
	}

	return chave, nil
}

func valorOuPadrao(valor, padrao string) string {
	if strings.TrimSpace(valor) == "" {
		return padrao
	}
	return valor
}

func clienteOuPadrao(cliente *http.Client) *http.Client {
	if cliente != nil {
		return cliente
	}
	return &http.Client{Timeout: tempoLimiteHTTP}
}

func relogioOuPadrao(agora func() time.Time) func() time.Time {
	if agora != nil {
		return agora
	}
	return time.Now
}

func esperaOuPadrao(espera func(time.Duration)) func(time.Duration) {
	if espera != nil {
		return espera
	}
	return time.Sleep
}
