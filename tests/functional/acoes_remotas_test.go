package funcional

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/api"
	"github.com/Azulpasta/aPRova/internal/platform"
)

const (
	segredoDasAcoes   = "segredo-compartilhado-com-o-n8n"
	instalacaoDeTeste = 7001
	autorDoPRDeTeste  = "carol"
)

var momentoDeTeste = time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)

type reviewRecebida struct {
	Evento string `json:"event"`
	Corpo  string `json:"body"`
}

type githubFalso struct {
	*httptest.Server

	mutex             sync.Mutex
	papelPorLogin     map[string]string
	statusDaPermissao int
	chamadasTotais    int
	chamadasDeEscrita int
	reviews           []reviewRecebida
}

func novoGitHubFalso(t *testing.T) *githubFalso {
	t.Helper()

	falso := &githubFalso{papelPorLogin: map[string]string{}, statusDaPermissao: http.StatusOK}
	falso.Server = httptest.NewServer(http.HandlerFunc(falso.atender))
	t.Cleanup(falso.Close)

	return falso
}

func (g *githubFalso) atender(w http.ResponseWriter, r *http.Request) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.chamadasTotais++
	if r.Method != http.MethodGet {
		g.chamadasDeEscrita++
	}

	partes := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	switch {
	case len(partes) == 6 && partes[3] == "collaborators" && partes[5] == "permission":
		g.responderPermissao(w, partes[4])
	case len(partes) == 5 && partes[3] == "pulls" && r.Method == http.MethodGet:
		responderJSON(w, map[string]any{"user": map[string]string{"login": autorDoPRDeTeste}})
	case len(partes) == 6 && partes[5] == "reviews" && r.Method == http.MethodPost:
		var review reviewRecebida
		_ = json.NewDecoder(r.Body).Decode(&review)
		g.reviews = append(g.reviews, review)
		responderJSON(w, map[string]any{"id": len(g.reviews)})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *githubFalso) responderPermissao(w http.ResponseWriter, login string) {
	if g.statusDaPermissao != http.StatusOK {
		w.WriteHeader(g.statusDaPermissao)
		return
	}

	papel, existe := g.papelPorLogin[login]
	if !existe {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	responderJSON(w, map[string]string{"permission": papel, "role_name": papel})
}

func (g *githubFalso) definirPapel(login, papel string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.papelPorLogin[login] = papel
}

func (g *githubFalso) falharConsultaDePermissao(status int) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.statusDaPermissao = status
}

func (g *githubFalso) escritasRecebidas() int {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.chamadasDeEscrita
}

func (g *githubFalso) chamadasRecebidas() int {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.chamadasTotais
}

func (g *githubFalso) reviewsRecebidas() []reviewRecebida {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return append([]reviewRecebida(nil), g.reviews...)
}

func responderJSON(w http.ResponseWriter, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(corpo)
}

type tokensFalsos struct{}

func (tokensFalsos) TokenParaInstalacao(context.Context, int64) (string, error) {
	return "token-de-instalacao-falso", nil
}

type ambiente struct {
	github     *githubFalso
	tentativas *acaoremota.RegistroEmMemoria
	rotas      http.Handler
}

func montarAmbiente(t *testing.T, vinculos map[string]string) *ambiente {
	t.Helper()

	github := novoGitHubFalso(t)
	clienteGitHub := platform.NovoCliente(platform.OpcoesCliente{
		Tokens:  tokensFalsos{},
		URLBase: github.URL,
	})
	tentativas := acaoremota.NovoRegistroEmMemoria()

	servico, err := acaoremota.NovoServico(acaoremota.OpcoesServico{
		Identidades: acaoremota.NovoResolvedorEmMemoria(vinculos),
		Permissoes:  clienteGitHub,
		Autoria:     clienteGitHub,
		Executor:    acaoremota.NovoExecutorNoGitHub(clienteGitHub, reenfileiradorFalso{}),
		Tentativas:  tentativas,
		Reservas:    acaoremota.NovaReservaEmMemoria(),
		Agora:       func() time.Time { return momentoDeTeste },
	})
	if err != nil {
		t.Fatalf("montar o serviço de ações remotas: %v", err)
	}

	rotas := api.NovoServidor(api.Opcoes{
		SegredoAcoesRemotas: []byte(segredoDasAcoes),
		AcoesRemotas:        servico,
		Agora:               func() time.Time { return momentoDeTeste },
	}).Rotas()

	return &ambiente{github: github, tentativas: tentativas, rotas: rotas}
}

type reenfileiradorFalso struct{}

func (reenfileiradorFalso) Reenfileirar(context.Context, acaoremota.AcaoAutorizada) error {
	return nil
}

func (a *ambiente) enviarAssinado(t *testing.T, corpo string) *httptest.ResponseRecorder {
	t.Helper()

	timestamp := strconv.FormatInt(momentoDeTeste.Unix(), 10)
	return a.enviar(t, corpo, timestamp, assinar(segredoDasAcoes, timestamp, corpo))
}

func (a *ambiente) enviar(t *testing.T, corpo, timestamp, assinatura string) *httptest.ResponseRecorder {
	t.Helper()

	requisicao := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/acoes-remotas", strings.NewReader(corpo))
	if timestamp != "" {
		requisicao.Header.Set("X-Aprova-Timestamp", timestamp)
	}
	if assinatura != "" {
		requisicao.Header.Set("X-Aprova-Assinatura", assinatura)
	}

	gravador := httptest.NewRecorder()
	a.rotas.ServeHTTP(gravador, requisicao)

	return gravador
}

func assinar(segredo, timestamp, corpo string) string {
	mac := hmac.New(sha256.New, []byte(segredo))
	mac.Write([]byte(timestamp + "." + corpo))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func corpoDeAcao(idAcao, tipo, usuarioSlack string) string {
	return `{"id_acao":"` + idAcao + `","tipo":"` + tipo + `","usuario_slack":"` + usuarioSlack + `",` +
		`"repositorio":"Azulpasta/aPRova","numero_pr":42,"installation_id":7001}`
}

func lerCorpo(t *testing.T, resposta *httptest.ResponseRecorder) string {
	t.Helper()

	bruto, err := io.ReadAll(resposta.Body)
	if err != nil {
		t.Fatalf("ler corpo da resposta: %v", err)
	}
	return string(bruto)
}

func TestUsuarioSemPermissaoDeEscritaNaoAprova(t *testing.T) {
	for _, papel := range []string{"read", "triage"} {
		t.Run(papel, func(t *testing.T) {
			verificarRecusaDePapelSemEscrita(t, papel)
		})
	}
}

func verificarRecusaDePapelSemEscrita(t *testing.T, papel string) {
	t.Helper()

	ambiente := montarAmbiente(t, map[string]string{"U-LEITOR": "dave"})
	ambiente.github.definirPapel("dave", papel)

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "aprovar", "U-LEITOR"))

	if resposta.Code != http.StatusForbidden {
		t.Fatalf("quem tem papel %s não pode aprovar: esperava 403, obtive %d", papel, resposta.Code)
	}
	if escritas := ambiente.github.escritasRecebidas(); escritas != 0 {
		t.Errorf("nenhuma chamada de escrita pode chegar ao GitHub numa recusa, chegaram %d", escritas)
	}

	tentativas := ambiente.tentativas.Tentativas()
	if len(tentativas) != 1 {
		t.Fatalf("a tentativa recusada precisa ficar registrada, há %d registros", len(tentativas))
	}
	registrada := tentativas[0]
	if registrada.Decisao != acaoremota.DecisaoRecusada || registrada.LoginGitHub != "dave" ||
		registrada.IDAcao != "acao-1" || registrada.Motivo != acaoremota.MotivoPermissaoInsuficiente {
		t.Errorf("registro incompleto ou errado: %+v", registrada)
	}

	if corpo := lerCorpo(t, resposta); strings.Contains(corpo, papel) || strings.Contains(corpo, "dave") {
		t.Errorf("a resposta não pode detalhar o motivo da recusa, devolveu %s", corpo)
	}
}

func TestPedidoSemAutenticacaoValidaNaoChegaAoGitHub(t *testing.T) {
	corpo := corpoDeAcao("acao-1", "aprovar", "U-ESCRITOR")
	agora := strconv.FormatInt(momentoDeTeste.Unix(), 10)
	velho := strconv.FormatInt(momentoDeTeste.Add(-6*time.Minute).Unix(), 10)

	casos := []struct {
		nome       string
		timestamp  string
		assinatura string
	}{
		{"assinatura ausente", agora, ""},
		{"assinatura inválida", agora, assinar("segredo-adivinhado", agora, corpo)},
		{"timestamp velho", velho, assinar(segredoDasAcoes, velho, corpo)},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
			ambiente.github.definirPapel("erin", "write")

			resposta := ambiente.enviar(t, corpo, caso.timestamp, caso.assinatura)

			if resposta.Code != http.StatusUnauthorized {
				t.Errorf("esperava 401, obtive %d", resposta.Code)
			}
			if chamadas := ambiente.github.chamadasRecebidas(); chamadas != 0 {
				t.Errorf("pedido não autenticado não pode chegar ao GitHub, chegaram %d chamadas", chamadas)
			}
			if registradas := len(ambiente.tentativas.Tentativas()); registradas != 0 {
				t.Errorf("só pedido autenticado gera registro, há %d", registradas)
			}
		})
	}
}

func TestUsuarioSlackSemVinculoEhRecusado(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{})

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "aprovar", "U-ESTRANHO"))

	if resposta.Code != http.StatusForbidden {
		t.Fatalf("usuário sem vínculo: esperava 403, obtive %d", resposta.Code)
	}
	tentativas := ambiente.tentativas.Tentativas()
	if len(tentativas) != 1 || tentativas[0].Motivo != acaoremota.MotivoSemVinculo || tentativas[0].UsuarioSlack != "U-ESTRANHO" {
		t.Errorf("a recusa por falta de vínculo precisa ficar registrada: %+v", tentativas)
	}
	if chamadas := ambiente.github.chamadasRecebidas(); chamadas != 0 {
		t.Errorf("sem login não há o que perguntar ao GitHub, foram %d chamadas", chamadas)
	}
}

func TestEscritorAprovaPRDeOutraPessoa(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "write")

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "aprovar", "U-ESCRITOR"))

	if resposta.Code != http.StatusAccepted {
		t.Fatalf("write aprovando PR de outra pessoa: esperava 202, obtive %d", resposta.Code)
	}
	reviews := ambiente.github.reviewsRecebidas()
	esperada := reviewRecebida{Evento: "APPROVE", Corpo: "Aprovado por @erin via Slack"}
	if len(reviews) != 1 || reviews[0] != esperada {
		t.Errorf("reviews = %+v, esperava só %+v", reviews, esperada)
	}
}

func TestAutorNaoAprovaOProprioPR(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-AUTORA": autorDoPRDeTeste})
	ambiente.github.definirPapel(autorDoPRDeTeste, "admin")

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "aprovar", "U-AUTORA"))

	if resposta.Code != http.StatusForbidden {
		t.Fatalf("a autora é admin mas não aprova o próprio PR: esperava 403, obtive %d", resposta.Code)
	}
	if escritas := ambiente.github.escritasRecebidas(); escritas != 0 {
		t.Errorf("nenhuma review pode ser enviada, chegaram %d escritas", escritas)
	}
}

func TestFalhaDoGitHubNaConsultaDePermissaoRecusa(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "admin")
	ambiente.github.falharConsultaDePermissao(http.StatusInternalServerError)

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "aprovar", "U-ESCRITOR"))

	if resposta.Code != http.StatusForbidden {
		t.Fatalf("falha na verificação nunca libera: esperava 403, obtive %d", resposta.Code)
	}
	if escritas := ambiente.github.escritasRecebidas(); escritas != 0 {
		t.Errorf("nenhuma review pode ser enviada, chegaram %d escritas", escritas)
	}
}

func TestSolicitarAlteracaoSemJustificativaEhCorpoInvalido(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "write")

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "solicitar_alteracao", "U-ESCRITOR"))

	if resposta.Code != http.StatusBadRequest {
		t.Errorf("solicitar alteração sem justificativa: esperava 400, obtive %d", resposta.Code)
	}
}

func TestSolicitarAlteracaoComJustificativaEnviaAReview(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "maintain")
	corpo := strings.TrimSuffix(corpoDeAcao("acao-1", "solicitar_alteracao", "U-ESCRITOR"), "}") +
		`,"justificativa":"falta teste de timeout"}`

	resposta := ambiente.enviarAssinado(t, corpo)

	if resposta.Code != http.StatusAccepted {
		t.Fatalf("maintain pedindo alteração: esperava 202, obtive %d", resposta.Code)
	}
	reviews := ambiente.github.reviewsRecebidas()
	esperada := reviewRecebida{Evento: "REQUEST_CHANGES", Corpo: "Alteração solicitada por @erin via Slack:\n\nfalta teste de timeout"}
	if len(reviews) != 1 || reviews[0] != esperada {
		t.Errorf("reviews = %+v, esperava só %+v", reviews, esperada)
	}
}

func TestIDAcaoRepetidoNaoGeraSegundaReview(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "write")
	corpo := corpoDeAcao("acao-1", "aprovar", "U-ESCRITOR")

	primeira := ambiente.enviarAssinado(t, corpo)
	segunda := ambiente.enviarAssinado(t, corpo)

	if primeira.Code != http.StatusAccepted || segunda.Code != http.StatusConflict {
		t.Fatalf("esperava 202 e depois 409, obtive %d e %d", primeira.Code, segunda.Code)
	}
	if reviews := ambiente.github.reviewsRecebidas(); len(reviews) != 1 {
		t.Errorf("clique duplo gerou %d reviews", len(reviews))
	}
}

func TestTipoDesconhecidoEhCorpoInvalido(t *testing.T) {
	ambiente := montarAmbiente(t, map[string]string{"U-ESCRITOR": "erin"})
	ambiente.github.definirPapel("erin", "admin")

	resposta := ambiente.enviarAssinado(t, corpoDeAcao("acao-1", "mesclar", "U-ESCRITOR"))

	if resposta.Code != http.StatusBadRequest {
		t.Errorf("tipo desconhecido: esperava 400, obtive %d", resposta.Code)
	}
	if chamadas := ambiente.github.chamadasRecebidas(); chamadas != 0 {
		t.Errorf("corpo inválido não chega ao GitHub, foram %d chamadas", chamadas)
	}
}
