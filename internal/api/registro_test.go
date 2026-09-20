package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func capturarLogJSON(t *testing.T) *bytes.Buffer {
	t.Helper()

	var saida bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&saida, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	return &saida
}

func TestRequisicoesSaoRegistradasEmJSONPeloSlog(t *testing.T) {
	saida := capturarLogJSON(t)

	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	NovoServidor(Opcoes{}).Rotas().ServeHTTP(httptest.NewRecorder(), requisicao)

	linhas := strings.Split(strings.TrimSpace(saida.String()), "\n")
	if len(linhas) == 0 || linhas[0] == "" {
		t.Fatal("a requisição deveria produzir ao menos uma linha de log")
	}

	var encontrouAcesso bool
	for _, linha := range linhas {
		var campos map[string]any
		if err := json.Unmarshal([]byte(linha), &campos); err != nil {
			t.Fatalf("linha de log não é JSON: %q", linha)
		}
		if campos["rota"] == "/health" && campos["metodo"] == http.MethodGet {
			encontrouAcesso = true
			if campos["status"] != float64(http.StatusOK) {
				t.Errorf("esperava status 200 no log de acesso, obtive %v", campos["status"])
			}
		}
	}

	if !encontrouAcesso {
		t.Errorf("esperava uma linha de acesso com rota e método, obtive: %s", saida.String())
	}
}
