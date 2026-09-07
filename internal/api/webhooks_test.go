package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookGitHubAceitaEDevolve202(t *testing.T) {
	gravador := httptest.NewRecorder()
	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/webhooks/github", strings.NewReader("{}"))

	NovoServidor().Rotas().ServeHTTP(gravador, requisicao)

	if gravador.Code != http.StatusAccepted {
		t.Errorf("webhook é aceito para processamento futuro; esperava 202, obtive %d", gravador.Code)
	}
}

func TestWebhookGitHubRecusaMetodoGet(t *testing.T) {
	gravador := httptest.NewRecorder()
	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/webhooks/github", nil)

	NovoServidor().Rotas().ServeHTTP(gravador, requisicao)

	if gravador.Code != http.StatusMethodNotAllowed {
		t.Errorf("esperava 405, obtive %d", gravador.Code)
	}
}
