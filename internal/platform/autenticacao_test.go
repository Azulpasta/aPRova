package platform

import (
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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	chaveDeTesteUmaVez sync.Once
	chaveDeTeste       *rsa.PrivateKey
)

func chavePrivadaDeTeste(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	chaveDeTesteUmaVez.Do(func() {
		gerada, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		chaveDeTeste = gerada
	})

	return chaveDeTeste
}

func pemDeTeste(t *testing.T) []byte {
	t.Helper()

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(chavePrivadaDeTeste(t)),
	})
}

type relogioFalso struct {
	mutex   sync.Mutex
	momento time.Time
}

func novoRelogioFalso() *relogioFalso {
	return &relogioFalso{momento: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)}
}

func (r *relogioFalso) agora() time.Time {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.momento
}

func (r *relogioFalso) avancar(duracao time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.momento = r.momento.Add(duracao)
}

type servidorDeTokens struct {
	*httptest.Server
	chamadas atomic.Int32
}

func novoServidorDeTokens(t *testing.T, relogio *relogioFalso) *servidorDeTokens {
	t.Helper()

	falso := &servidorDeTokens{}
	falso.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		numero := falso.chamadas.Add(1)
		expiraEm := relogio.agora().Add(time.Hour).Format(time.RFC3339)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "14999")
		w.Header().Set("X-RateLimit-Limit", "15000")
		_, _ = fmt.Fprintf(w, `{"token":"ghs_token_%d","expires_at":%q}`, numero, expiraEm)
	}))
	t.Cleanup(falso.Close)

	return falso
}

func autenticadorDeTeste(t *testing.T, urlBase string, relogio *relogioFalso) *Autenticador {
	t.Helper()

	autenticador, err := NovoAutenticador(OpcoesAutenticador{
		AppID:           "123456",
		ChavePrivadaPEM: pemDeTeste(t),
		URLBase:         urlBase,
		Agora:           relogio.agora,
		Espera:          func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("construir autenticador: %v", err)
	}

	return autenticador
}

func TestTokenEhRenovadoAntesDeExpirar(t *testing.T) {
	relogio := novoRelogioFalso()
	servidor := novoServidorDeTokens(t, relogio)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	primeiro, err := autenticador.TokenParaInstalacao(t.Context(), 42)
	if err != nil {
		t.Fatalf("primeiro token: %v", err)
	}

	relogio.avancar(56 * time.Minute)

	segundo, err := autenticador.TokenParaInstalacao(t.Context(), 42)
	if err != nil {
		t.Fatalf("segundo token: %v", err)
	}

	if servidor.chamadas.Load() != 2 {
		t.Errorf("faltando 4 minutos para expirar, o token deveria ser renovado; o github foi chamado %d vez(es)",
			servidor.chamadas.Load())
	}
	if primeiro == segundo {
		t.Error("a renovação deveria devolver um token novo, não o vencido")
	}
}

func servidorComAtraso(t *testing.T, relogio *relogioFalso, atraso time.Duration) *servidorDeTokens {
	t.Helper()

	falso := &servidorDeTokens{}
	falso.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		numero := falso.chamadas.Add(1)
		time.Sleep(atraso)
		expiraEm := relogio.agora().Add(time.Hour).Format(time.RFC3339)
		_, _ = fmt.Fprintf(w, `{"token":"ghs_token_%d","expires_at":%q}`, numero, expiraEm)
	}))
	t.Cleanup(falso.Close)

	return falso
}

func servidorQueResponde(t *testing.T, status int) *servidorDeTokens {
	t.Helper()

	falso := &servidorDeTokens{}
	falso.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		falso.chamadas.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(falso.Close)

	return falso
}

func partesDoJWT(t *testing.T, assinado string) (cabecalho, reivindicacoes map[string]any, conteudo, assinatura string) {
	t.Helper()

	partes := strings.Split(assinado, ".")
	if len(partes) != 3 {
		t.Fatalf("jwt deveria ter três partes, tem %d", len(partes))
	}

	decodificar := func(parte string) map[string]any {
		bruto, err := base64.RawURLEncoding.DecodeString(parte)
		if err != nil {
			t.Fatalf("parte do jwt não é base64url: %v", err)
		}
		var conteudo map[string]any
		if err := json.Unmarshal(bruto, &conteudo); err != nil {
			t.Fatalf("parte do jwt não é JSON: %v", err)
		}
		return conteudo
	}

	return decodificar(partes[0]), decodificar(partes[1]), partes[0] + "." + partes[1], partes[2]
}

func reivindicacaoNumerica(t *testing.T, reivindicacoes map[string]any, nome string) int64 {
	t.Helper()

	valor, ehNumero := reivindicacoes[nome].(float64)
	if !ehNumero {
		t.Fatalf("a reivindicação %q deveria ser numérica, obtive %T", nome, reivindicacoes[nome])
	}

	return int64(valor)
}

func TestTokenEmCacheEValidoEReutilizado(t *testing.T) {
	relogio := novoRelogioFalso()
	servidor := novoServidorDeTokens(t, relogio)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	primeiro, err := autenticador.TokenParaInstalacao(t.Context(), 42)
	if err != nil {
		t.Fatalf("primeiro token: %v", err)
	}

	relogio.avancar(30 * time.Minute)

	segundo, err := autenticador.TokenParaInstalacao(t.Context(), 42)
	if err != nil {
		t.Fatalf("segundo token: %v", err)
	}

	if servidor.chamadas.Load() != 1 {
		t.Errorf("faltando 30 minutos para expirar, o token deveria vir do cache; houve %d chamada(s)",
			servidor.chamadas.Load())
	}
	if primeiro != segundo {
		t.Error("o token do cache deveria ser o mesmo")
	}
}

func TestJWTCarregaIssIatEExpDentroDoLimite(t *testing.T) {
	relogio := novoRelogioFalso()
	autenticador := autenticadorDeTeste(t, "http://exemplo.invalido", relogio)

	assinado, err := autenticador.gerarJWT()
	if err != nil {
		t.Fatalf("gerar jwt: %v", err)
	}

	_, reivindicacoes, _, _ := partesDoJWT(t, assinado)

	if reivindicacoes["iss"] != "123456" {
		t.Errorf("iss deveria ser o app id, obtive %v", reivindicacoes["iss"])
	}

	agora := relogio.agora()
	emitidoEm := reivindicacaoNumerica(t, reivindicacoes, "iat")
	expiraEm := reivindicacaoNumerica(t, reivindicacoes, "exp")

	if emitidoEm != agora.Add(-toleranciaDeRelogio).Unix() {
		t.Errorf("iat deveria recuar 60s para tolerar relógio dessincronizado, obtive %d", emitidoEm)
	}
	if expiraEm <= agora.Unix() {
		t.Error("exp deveria estar no futuro")
	}
	if expiraEm > agora.Add(10*time.Minute).Unix() {
		t.Errorf("exp não pode passar de 10 minutos, excedeu em %ds", expiraEm-agora.Add(10*time.Minute).Unix())
	}
}

func TestJWTEAssinadoComRS256EVerificaContraAChavePublica(t *testing.T) {
	relogio := novoRelogioFalso()
	autenticador := autenticadorDeTeste(t, "http://exemplo.invalido", relogio)

	assinado, err := autenticador.gerarJWT()
	if err != nil {
		t.Fatalf("gerar jwt: %v", err)
	}

	cabecalho, _, conteudo, assinaturaEmBase64 := partesDoJWT(t, assinado)

	if cabecalho["alg"] != "RS256" {
		t.Errorf("alg deveria ser RS256, obtive %v", cabecalho["alg"])
	}

	assinatura, err := base64.RawURLEncoding.DecodeString(assinaturaEmBase64)
	if err != nil {
		t.Fatalf("assinatura não é base64url: %v", err)
	}

	resumo := sha256.Sum256([]byte(conteudo))
	if err := rsa.VerifyPKCS1v15(&chavePrivadaDeTeste(t).PublicKey, crypto.SHA256, resumo[:], assinatura); err != nil {
		t.Errorf("a assinatura deveria verificar contra a chave pública correspondente: %v", err)
	}
}

func TestChavePrivadaMalformadaFalhaNaInicializacao(t *testing.T) {
	_, err := NovoAutenticador(OpcoesAutenticador{
		AppID:           "123456",
		ChavePrivadaPEM: []byte("isto não é uma chave"),
	})

	if err == nil {
		t.Fatal("chave malformada deveria falhar na inicialização, não na primeira chamada")
	}
	if !errors.Is(err, ErrConfiguracaoInvalida) {
		t.Errorf("o erro deveria ser classificado como configuração inválida, obtive: %v", err)
	}
}

func TestPedidosSimultaneosGeramUmaUnicaChamadaAoGitHub(t *testing.T) {
	const pedidosConcorrentes = 20

	relogio := novoRelogioFalso()
	servidor := servidorComAtraso(t, relogio, 40*time.Millisecond)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	tokens := make([]string, pedidosConcorrentes)
	erros := make([]error, pedidosConcorrentes)

	var grupo sync.WaitGroup
	for indice := range pedidosConcorrentes {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			tokens[indice], erros[indice] = autenticador.TokenParaInstalacao(t.Context(), 42)
		}()
	}
	grupo.Wait()

	for indice, err := range erros {
		if err != nil {
			t.Fatalf("pedido %d falhou: %v", indice, err)
		}
	}

	if servidor.chamadas.Load() != 1 {
		t.Errorf("%d pedidos simultâneos deveriam gerar 1 chamada ao github, geraram %d",
			pedidosConcorrentes, servidor.chamadas.Load())
	}
	for indice, token := range tokens {
		if token != tokens[0] {
			t.Errorf("todos deveriam receber o mesmo token; o %d divergiu", indice)
		}
	}
}

func TestRespostaQuatrocentosEQuatroIndicaAppNaoInstalado(t *testing.T) {
	relogio := novoRelogioFalso()
	servidor := servidorQueResponde(t, http.StatusNotFound)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	_, err := autenticador.TokenParaInstalacao(t.Context(), 42)

	if !errors.Is(err, ErrAppNaoInstalado) {
		t.Errorf("404 deveria indicar app não instalado, obtive: %v", err)
	}
	if errors.Is(err, ErrGitHubIndisponivel) {
		t.Error("404 não pode ser confundido com indisponibilidade transitória")
	}
	if servidor.chamadas.Load() != 1 {
		t.Errorf("404 não é transitório e não deve ser repetido; houve %d chamada(s)", servidor.chamadas.Load())
	}
}

func TestRespostaCincoXXEhTransitoriaERepetida(t *testing.T) {
	relogio := novoRelogioFalso()
	servidor := servidorQueResponde(t, http.StatusBadGateway)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	_, err := autenticador.TokenParaInstalacao(t.Context(), 42)

	if !errors.Is(err, ErrGitHubIndisponivel) {
		t.Errorf("5xx deveria ser classificado como transitório, obtive: %v", err)
	}
	if errors.Is(err, ErrAppNaoInstalado) {
		t.Error("5xx não pode ser confundido com app não instalado")
	}
	if servidor.chamadas.Load() < 2 {
		t.Errorf("5xx é transitório e merece nova tentativa; houve apenas %d chamada(s)", servidor.chamadas.Load())
	}
}

func TestRespostaNaoAutorizadaEhRepetidaUmaVez(t *testing.T) {
	relogio := novoRelogioFalso()
	servidor := servidorQueResponde(t, http.StatusUnauthorized)
	autenticador := autenticadorDeTeste(t, servidor.URL, relogio)

	_, err := autenticador.TokenParaInstalacao(t.Context(), 42)

	if !errors.Is(err, ErrCredencialRejeitada) {
		t.Errorf("401 deveria ser classificado como credencial rejeitada, obtive: %v", err)
	}
	if servidor.chamadas.Load() < 2 {
		t.Errorf("401 costuma ser relógio dessincronizado e merece uma nova tentativa; houve %d", servidor.chamadas.Load())
	}
}
