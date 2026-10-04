package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
)

const segredoDasAcoesDeTeste = "segredo-das-acoes-de-teste"

var agoraDeTeste = time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)

type processadorFalso struct {
	decisao  acaoremota.Decisao
	recebido [][]byte
}

func (p *processadorFalso) Processar(_ context.Context, corpo []byte) acaoremota.Decisao {
	p.recebido = append(p.recebido, corpo)
	return p.decisao
}

type pedidoAssinado struct {
	corpo      string
	timestamp  string
	assinatura string
}

func assinarAcao(segredo, timestamp, corpo string) string {
	mac := hmac.New(sha256.New, []byte(segredo))
	mac.Write([]byte(timestamp + "." + corpo))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func pedidoValido(corpo string) pedidoAssinado {
	timestamp := strconv.FormatInt(agoraDeTeste.Unix(), 10)
	return pedidoAssinado{
		corpo:      corpo,
		timestamp:  timestamp,
		assinatura: assinarAcao(segredoDasAcoesDeTeste, timestamp, corpo),
	}
}

func enviarAcao(t *testing.T, segredo string, processador *processadorFalso, pedido pedidoAssinado) *httptest.ResponseRecorder {
	t.Helper()

	servidor := NovoServidor(Opcoes{
		SegredoAcoesRemotas: []byte(segredo),
		AcoesRemotas:        processador,
		Agora:               func() time.Time { return agoraDeTeste },
	})

	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/acoes-remotas", strings.NewReader(pedido.corpo))
	if pedido.timestamp != "" {
		requisicao.Header.Set("X-Aprova-Timestamp", pedido.timestamp)
	}
	if pedido.assinatura != "" {
		requisicao.Header.Set("X-Aprova-Assinatura", pedido.assinatura)
	}

	gravador := httptest.NewRecorder()
	servidor.Rotas().ServeHTTP(gravador, requisicao)

	return gravador
}

const corpoDeAcaoDeTeste = `{"id_acao":"acao-1","tipo":"aprovar"}`

func TestAcoesRemotasRecusaPedidoSemAutenticacaoValida(t *testing.T) {
	timestampAtual := strconv.FormatInt(agoraDeTeste.Unix(), 10)
	timestampVelho := strconv.FormatInt(agoraDeTeste.Add(-5*time.Minute-time.Second).Unix(), 10)
	timestampFuturo := strconv.FormatInt(agoraDeTeste.Add(5*time.Minute+time.Second).Unix(), 10)

	casos := []struct {
		nome   string
		pedido pedidoAssinado
	}{
		{"sem assinatura", pedidoAssinado{corpo: corpoDeAcaoDeTeste, timestamp: timestampAtual}},
		{"sem timestamp", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, "", corpoDeAcaoDeTeste),
		}},
		{"assinatura com outro segredo", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampAtual,
			assinatura: assinarAcao("outro-segredo", timestampAtual, corpoDeAcaoDeTeste),
		}},
		{"assinatura sem o prefixo", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampAtual,
			assinatura: strings.TrimPrefix(assinarAcao(segredoDasAcoesDeTeste, timestampAtual, corpoDeAcaoDeTeste), "sha256="),
		}},
		{"assinatura só do corpo, sem o timestamp", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampAtual,
			assinatura: assinaturaSoDoCorpo(corpoDeAcaoDeTeste),
		}},
		{"corpo alterado depois de assinado", pedidoAssinado{
			corpo:      `{"id_acao":"acao-1","tipo":"solicitar_alteracao"}`,
			timestamp:  timestampAtual,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, timestampAtual, corpoDeAcaoDeTeste),
		}},
		{"timestamp trocado por um recente", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampAtual,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, timestampVelho, corpoDeAcaoDeTeste),
		}},
		{"timestamp com mais de 5 minutos", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampVelho,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, timestampVelho, corpoDeAcaoDeTeste),
		}},
		{"timestamp mais de 5 minutos no futuro", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestampFuturo,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, timestampFuturo, corpoDeAcaoDeTeste),
		}},
		{"timestamp que não é número", pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  "ontem",
			assinatura: assinarAcao(segredoDasAcoesDeTeste, "ontem", corpoDeAcaoDeTeste),
		}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			processador := &processadorFalso{decisao: acaoremota.DecisaoAceita}

			resposta := enviarAcao(t, segredoDasAcoesDeTeste, processador, caso.pedido)

			if resposta.Code != http.StatusUnauthorized {
				t.Errorf("esperava 401, obtive %d", resposta.Code)
			}
			if len(processador.recebido) != 0 {
				t.Error("pedido não autenticado não pode chegar ao processador")
			}
		})
	}
}

func assinaturaSoDoCorpo(corpo string) string {
	mac := hmac.New(sha256.New, []byte(segredoDasAcoesDeTeste))
	mac.Write([]byte(corpo))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestAcoesRemotasAceitaTimestampNoLimiteDaJanela(t *testing.T) {
	for _, deslocamento := range []time.Duration{-5 * time.Minute, 5 * time.Minute} {
		timestamp := strconv.FormatInt(agoraDeTeste.Add(deslocamento).Unix(), 10)
		processador := &processadorFalso{decisao: acaoremota.DecisaoAceita}

		resposta := enviarAcao(t, segredoDasAcoesDeTeste, processador, pedidoAssinado{
			corpo:      corpoDeAcaoDeTeste,
			timestamp:  timestamp,
			assinatura: assinarAcao(segredoDasAcoesDeTeste, timestamp, corpoDeAcaoDeTeste),
		})

		if resposta.Code != http.StatusAccepted {
			t.Errorf("deslocamento de %v ainda está na janela, obtive %d", deslocamento, resposta.Code)
		}
	}
}

func TestAcoesRemotasRecusaTudoQuandoOSegredoNaoFoiConfigurado(t *testing.T) {
	processador := &processadorFalso{decisao: acaoremota.DecisaoAceita}
	timestamp := strconv.FormatInt(agoraDeTeste.Unix(), 10)

	resposta := enviarAcao(t, "", processador, pedidoAssinado{
		corpo:      corpoDeAcaoDeTeste,
		timestamp:  timestamp,
		assinatura: assinarAcao("", timestamp, corpoDeAcaoDeTeste),
	})

	if resposta.Code != http.StatusUnauthorized {
		t.Errorf("segredo vazio é segredo que qualquer um conhece: esperava 401, obtive %d", resposta.Code)
	}
	if len(processador.recebido) != 0 {
		t.Error("sem segredo configurado nada pode chegar ao processador")
	}
}

func TestAcoesRemotasEntregaOCorpoBrutoAoProcessador(t *testing.T) {
	processador := &processadorFalso{decisao: acaoremota.DecisaoAceita}

	enviarAcao(t, segredoDasAcoesDeTeste, processador, pedidoValido(corpoDeAcaoDeTeste))

	if len(processador.recebido) != 1 || string(processador.recebido[0]) != corpoDeAcaoDeTeste {
		t.Errorf("o processador precisa receber exatamente o corpo assinado, recebeu %q", processador.recebido)
	}
}

func TestAcoesRemotasTraduzADecisaoEmStatus(t *testing.T) {
	casos := []struct {
		decisao acaoremota.Decisao
		status  int
		corpo   string
	}{
		{acaoremota.DecisaoAceita, http.StatusAccepted, `{"status":"aceito"}`},
		{acaoremota.DecisaoRecusada, http.StatusForbidden, `{"status":"sem permissão"}`},
		{acaoremota.DecisaoInvalida, http.StatusBadRequest, `{"status":"corpo inválido"}`},
		{acaoremota.DecisaoRepetida, http.StatusConflict, `{"status":"ação já processada"}`},
		{acaoremota.DecisaoFalha, http.StatusInternalServerError, `{"status":"falha ao executar"}`},
		{"desconhecida", http.StatusInternalServerError, `{"status":"falha ao executar"}`},
	}

	for _, caso := range casos {
		t.Run(string(caso.decisao), func(t *testing.T) {
			resposta := enviarAcao(t, segredoDasAcoesDeTeste, &processadorFalso{decisao: caso.decisao}, pedidoValido(corpoDeAcaoDeTeste))

			if resposta.Code != caso.status {
				t.Errorf("status = %d, esperava %d", resposta.Code, caso.status)
			}
			if corpo := strings.TrimSpace(resposta.Body.String()); corpo != caso.corpo {
				t.Errorf("corpo = %s, esperava %s", corpo, caso.corpo)
			}
		})
	}
}

func TestAcoesRemotasNaoRegistraSegredoNemAssinatura(t *testing.T) {
	var saida bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&saida, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	timestamp := strconv.FormatInt(agoraDeTeste.Unix(), 10)
	assinaturaErrada := assinarAcao("outro-segredo", timestamp, corpoDeAcaoDeTeste)

	enviarAcao(t, segredoDasAcoesDeTeste, &processadorFalso{}, pedidoAssinado{
		corpo: corpoDeAcaoDeTeste, timestamp: timestamp, assinatura: assinaturaErrada,
	})
	valido := pedidoValido(corpoDeAcaoDeTeste)
	enviarAcao(t, segredoDasAcoesDeTeste, &processadorFalso{decisao: acaoremota.DecisaoRecusada}, valido)

	registro := saida.String()
	if !strings.Contains(registro, "assinatura") {
		t.Fatalf("a recusa por assinatura precisa aparecer no log, obtive:\n%s", registro)
	}
	for _, sensivel := range []string{segredoDasAcoesDeTeste, assinaturaErrada, valido.assinatura} {
		if strings.Contains(registro, strings.TrimPrefix(sensivel, "sha256=")) {
			t.Errorf("o log contém material sensível %q:\n%s", sensivel, registro)
		}
	}
}
