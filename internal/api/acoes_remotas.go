package api

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
)

const (
	cabecalhoAssinaturaDaAcao = "X-Aprova-Assinatura"
	cabecalhoTimestampDaAcao  = "X-Aprova-Timestamp"

	janelaDeValidadeDaAcao = 5 * time.Minute
)

type respostaDaDecisao struct {
	status   int
	mensagem string
}

var respostaDeFalha = respostaDaDecisao{http.StatusInternalServerError, "falha ao executar"}

var respostasPorDecisao = map[acaoremota.Decisao]respostaDaDecisao{
	acaoremota.DecisaoAceita:   {http.StatusAccepted, "aceito"},
	acaoremota.DecisaoRecusada: {http.StatusForbidden, "sem permissão"},
	acaoremota.DecisaoInvalida: {http.StatusBadRequest, "corpo inválido"},
	acaoremota.DecisaoRepetida: {http.StatusConflict, "ação já processada"},
	acaoremota.DecisaoFalha:    respostaDeFalha,
}

func (s *Servidor) manipularAcaoRemota(w http.ResponseWriter, r *http.Request) {
	corpoBruto, err := io.ReadAll(http.MaxBytesReader(w, r.Body, tamanhoMaximoDoCorpo))
	if err != nil {
		slog.Warn("corpo da ação remota não pôde ser lido")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if motivo := s.motivoParaNaoAutenticar(r.Header, corpoBruto); motivo != "" {
		slog.Warn("ação remota recusada na autenticação", "motivo", motivo)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	resposta, conhecida := respostasPorDecisao[s.acoesRemotas.Processar(r.Context(), corpoBruto)]
	if !conhecida {
		resposta = respostaDeFalha
	}

	responderJSON(w, resposta.status, respostaSimples{Status: resposta.mensagem})
}

func (s *Servidor) motivoParaNaoAutenticar(cabecalhos http.Header, corpoBruto []byte) string {
	if len(s.segredoAcoesRemotas) == 0 || s.acoesRemotas == nil {
		return "ações remotas não configuradas"
	}

	timestamp := cabecalhos.Get(cabecalhoTimestampDaAcao)
	segundos, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "timestamp ausente ou inválido"
	}

	conteudoAssinado := append([]byte(timestamp+"."), corpoBruto...)
	if !assinaturaConfere(s.segredoAcoesRemotas, conteudoAssinado, cabecalhos.Get(cabecalhoAssinaturaDaAcao)) {
		return "assinatura não confere"
	}

	if s.agora().Sub(time.Unix(segundos, 0)).Abs() > janelaDeValidadeDaAcao {
		return "timestamp fora da janela de 5 minutos"
	}

	return ""
}
