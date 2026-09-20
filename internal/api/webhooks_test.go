package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Azulpasta/aPRova/internal/domain"
)

const segredoDeTeste = "segredo-de-teste"

type registroFalso struct {
	execucoes  map[string]domain.EventoPullRequest
	porEntrega map[string]string
	revisoes   []domain.DecisaoDeReview
}

func novoRegistroFalso() *registroFalso {
	return &registroFalso{
		execucoes:  map[string]domain.EventoPullRequest{},
		porEntrega: map[string]string{},
	}
}

func (r *registroFalso) RegistrarPullRequest(_ context.Context, evento domain.EventoPullRequest) (domain.Execucao, error) {
	if id, repetida := r.porEntrega[evento.DeliveryID]; repetida {
		return domain.Execucao{ID: id, JaRegistrada: true}, nil
	}

	id := evento.DeliveryID + "-execucao"
	r.porEntrega[evento.DeliveryID] = id
	r.execucoes[id] = evento

	return domain.Execucao{ID: id}, nil
}

func (r *registroFalso) RegistrarRevisao(_ context.Context, decisao domain.DecisaoDeReview) error {
	r.revisoes = append(r.revisoes, decisao)
	return nil
}

func (r *registroFalso) quantidadeDeExecucoes() int {
	return len(r.execucoes)
}

func assinarComSegredoDeTeste(corpo string) string {
	mac := hmac.New(sha256.New, []byte(segredoDeTeste))
	mac.Write([]byte(corpo))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type entrega struct {
	evento     string
	deliveryID string
	assinatura string
	corpo      string
}

func enviarWebhook(t *testing.T, registro *registroFalso, e entrega) *httptest.ResponseRecorder {
	t.Helper()

	servidor := NovoServidor(Opcoes{
		SegredoWebhook: []byte(segredoDeTeste),
		Execucoes:      registro,
	})

	requisicao := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/webhooks/github", strings.NewReader(e.corpo))
	requisicao.Header.Set("X-GitHub-Event", e.evento)
	requisicao.Header.Set("X-GitHub-Delivery", e.deliveryID)
	if e.assinatura != "" {
		requisicao.Header.Set("X-Hub-Signature-256", e.assinatura)
	}

	gravador := httptest.NewRecorder()
	servidor.Rotas().ServeHTTP(gravador, requisicao)

	return gravador
}

func corpoPullRequest(acao string) string {
	return `{"action":"` + acao + `","number":42,` +
		`"pull_request":{"head":{"sha":"abc123"},"user":{"login":"alice"}},` +
		`"repository":{"full_name":"Azulpasta/aPRova"}}`
}

func entregaValida(evento, deliveryID, corpo string) entrega {
	return entrega{
		evento:     evento,
		deliveryID: deliveryID,
		assinatura: assinarComSegredoDeTeste(corpo),
		corpo:      corpo,
	}
}

func TestWebhookRejeitaAssinaturaInvalida(t *testing.T) {
	registro := novoRegistroFalso()
	corpo := corpoPullRequest("opened")

	resposta := enviarWebhook(t, registro, entrega{
		evento:     "pull_request",
		deliveryID: "entrega-1",
		assinatura: "sha256=0000000000000000000000000000000000000000000000000000000000000000",
		corpo:      corpo,
	})

	if resposta.Code != http.StatusUnauthorized {
		t.Errorf("assinatura inválida deve ser rejeitada com 401, obtive %d", resposta.Code)
	}
	if registro.quantidadeDeExecucoes() != 0 {
		t.Error("nenhuma execução pode ser criada quando a assinatura não confere")
	}
}

func TestWebhookRejeitaAssinaturaAusente(t *testing.T) {
	registro := novoRegistroFalso()

	resposta := enviarWebhook(t, registro, entrega{
		evento:     "pull_request",
		deliveryID: "entrega-2",
		corpo:      corpoPullRequest("opened"),
	})

	if resposta.Code != http.StatusUnauthorized {
		t.Errorf("cabeçalho de assinatura ausente deve ser rejeitado com 401, obtive %d", resposta.Code)
	}
	if registro.quantidadeDeExecucoes() != 0 {
		t.Error("nenhuma execução pode ser criada sem assinatura")
	}
}

func TestWebhookAceitaAssinaturaValida(t *testing.T) {
	registro := novoRegistroFalso()

	resposta := enviarWebhook(t, registro, entregaValida("pull_request", "entrega-3", corpoPullRequest("opened")))

	if resposta.Code != http.StatusAccepted {
		t.Errorf("assinatura válida com ação aceita deve responder 202, obtive %d", resposta.Code)
	}
}

func (r *registroFalso) quantidadeDeRevisoes() int {
	return len(r.revisoes)
}

func corpoReview(estado string) string {
	return `{"action":"submitted",` +
		`"review":{"state":"` + estado + `","user":{"login":"bob"}},` +
		`"pull_request":{"number":42,"head":{"sha":"abc123"}},` +
		`"repository":{"full_name":"Azulpasta/aPRova"}}`
}

func TestWebhookIgnoraTipoDeEventoNaoTratado(t *testing.T) {
	registro := novoRegistroFalso()
	corpo := `{"action":"opened"}`

	resposta := enviarWebhook(t, registro, entregaValida("issues", "entrega-4", corpo))

	if resposta.Code != http.StatusNoContent {
		t.Errorf("evento não tratado deve responder 204, obtive %d", resposta.Code)
	}
	if registro.quantidadeDeExecucoes() != 0 {
		t.Error("evento não tratado não pode criar execução")
	}
}

func TestWebhookIgnoraAcaoDePullRequestNaoAceita(t *testing.T) {
	for _, acao := range []string{"labeled", "edited", "assigned", "review_requested", "milestoned"} {
		t.Run(acao, func(t *testing.T) {
			registro := novoRegistroFalso()
			corpo := corpoPullRequest(acao)

			resposta := enviarWebhook(t, registro, entregaValida("pull_request", "entrega-"+acao, corpo))

			if resposta.Code != http.StatusNoContent {
				t.Errorf("ação %q deve responder 204, obtive %d", acao, resposta.Code)
			}
			if registro.quantidadeDeExecucoes() != 0 {
				t.Errorf("ação %q não pode criar execução", acao)
			}
		})
	}
}

func TestWebhookAceitaAsDuasAcoesDePullRequest(t *testing.T) {
	for _, acao := range []string{"opened", "synchronize"} {
		t.Run(acao, func(t *testing.T) {
			registro := novoRegistroFalso()
			corpo := corpoPullRequest(acao)

			resposta := enviarWebhook(t, registro, entregaValida("pull_request", "entrega-"+acao, corpo))

			if resposta.Code != http.StatusAccepted {
				t.Errorf("ação %q deve responder 202, obtive %d", acao, resposta.Code)
			}
			if registro.quantidadeDeExecucoes() != 1 {
				t.Errorf("ação %q deveria criar exatamente uma execução, criou %d", acao, registro.quantidadeDeExecucoes())
			}
		})
	}
}

func TestWebhookRegistraRevisaoQueDecide(t *testing.T) {
	for _, estado := range []string{"approved", "changes_requested"} {
		t.Run(estado, func(t *testing.T) {
			registro := novoRegistroFalso()
			corpo := corpoReview(estado)

			resposta := enviarWebhook(t, registro, entregaValida("pull_request_review", "entrega-"+estado, corpo))

			if resposta.Code != http.StatusAccepted {
				t.Errorf("review %q deve responder 202, obtive %d", estado, resposta.Code)
			}
			if registro.quantidadeDeRevisoes() != 1 {
				t.Errorf("review %q deveria registrar uma decisão, registrou %d", estado, registro.quantidadeDeRevisoes())
			}
		})
	}
}

func TestWebhookIgnoraRevisaoComentadaQueNaoDecide(t *testing.T) {
	registro := novoRegistroFalso()
	corpo := corpoReview("commented")

	resposta := enviarWebhook(t, registro, entregaValida("pull_request_review", "entrega-5", corpo))

	if resposta.Code != http.StatusNoContent {
		t.Errorf("review sem decisão deve responder 204, obtive %d", resposta.Code)
	}
	if registro.quantidadeDeRevisoes() != 0 {
		t.Error("review comentada não pode registrar decisão: tiraria a PR da espera sem motivo")
	}
}

func TestWebhookNaoDuplicaRegistroNaReentregaDoMesmoEvento(t *testing.T) {
	registro := novoRegistroFalso()
	corpo := corpoPullRequest("opened")
	mesmaEntrega := entregaValida("pull_request", "entrega-repetida", corpo)

	primeira := enviarWebhook(t, registro, mesmaEntrega)
	segunda := enviarWebhook(t, registro, mesmaEntrega)

	if primeira.Code != http.StatusAccepted {
		t.Errorf("primeira entrega deve responder 202, obtive %d", primeira.Code)
	}
	if segunda.Code != http.StatusAccepted {
		t.Errorf("reentrega também deve responder 202 para o GitHub parar de reenviar, obtive %d", segunda.Code)
	}
	if registro.quantidadeDeExecucoes() != 1 {
		t.Errorf("o mesmo delivery ID deve criar exatamente uma execução, criou %d", registro.quantidadeDeExecucoes())
	}
}
