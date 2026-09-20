package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/Azulpasta/aPRova/internal/domain"
)

const (
	cabecalhoEvento  = "X-GitHub-Event"
	cabecalhoEntrega = "X-GitHub-Delivery"

	eventoPullRequest       = "pull_request"
	eventoPullRequestReview = "pull_request_review"

	tamanhoMaximoDoCorpo = 1 << 20
)

var (
	acoesDePullRequestAceitas = []string{"opened", "synchronize"}
	estadosDeReviewQueDecidem = []string{"approved", "changes_requested"}
)

type entregaRecebida struct {
	deliveryID string
	corpoBruto []byte
	diario     *slog.Logger
}

type cargaPullRequest struct {
	Acao        string `json:"action"`
	Numero      int    `json:"number"`
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Usuario struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
	Repositorio struct {
		NomeCompleto string `json:"full_name"`
	} `json:"repository"`
}

type cargaReview struct {
	Review struct {
		Estado  string `json:"state"`
		Usuario struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"review"`
	PullRequest struct {
		Numero int `json:"number"`
	} `json:"pull_request"`
	Repositorio struct {
		NomeCompleto string `json:"full_name"`
	} `json:"repository"`
}

func (s *Servidor) manipularWebhookGitHub(w http.ResponseWriter, r *http.Request) {
	deliveryID := r.Header.Get(cabecalhoEntrega)
	tipoDeEvento := r.Header.Get(cabecalhoEvento)
	diario := slog.With("delivery_id", deliveryID, "evento", tipoDeEvento)

	corpoBruto, err := io.ReadAll(http.MaxBytesReader(w, r.Body, tamanhoMaximoDoCorpo))
	if err != nil {
		diario.Warn("corpo do webhook não pôde ser lido")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !assinaturaConfere(s.segredoWebhook, corpoBruto, r.Header.Get(cabecalhoAssinatura)) {
		diario.Warn("assinatura do webhook não confere")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	entrega := entregaRecebida{deliveryID: deliveryID, corpoBruto: corpoBruto, diario: diario}

	var status int
	switch tipoDeEvento {
	case eventoPullRequest:
		status, err = s.processarPullRequest(r.Context(), entrega)
	case eventoPullRequestReview:
		status, err = s.processarRevisao(r.Context(), entrega)
	default:
		diario.Debug("tipo de evento não tratado")
		status = http.StatusNoContent
	}

	if err != nil {
		diario.Error("falha ao processar webhook", "erro", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status == http.StatusAccepted {
		responderJSON(w, status, respostaSimples{Status: "aceito"})
		return
	}
	w.WriteHeader(status)
}

func (s *Servidor) processarPullRequest(ctx context.Context, entrega entregaRecebida) (int, error) {
	var carga cargaPullRequest
	if err := json.Unmarshal(entrega.corpoBruto, &carga); err != nil {
		entrega.diario.Warn("corpo do webhook não é JSON válido")
		return http.StatusBadRequest, nil
	}

	if !slices.Contains(acoesDePullRequestAceitas, carga.Acao) {
		entrega.diario.Debug("ação de pull request ignorada", "acao", carga.Acao)
		return http.StatusNoContent, nil
	}

	execucao, err := s.execucoes.RegistrarPullRequest(ctx, domain.EventoPullRequest{
		DeliveryID:  entrega.deliveryID,
		Repositorio: carga.Repositorio.NomeCompleto,
		NumeroPR:    carga.Numero,
		SHAHead:     carga.PullRequest.Head.SHA,
		Autor:       carga.PullRequest.Usuario.Login,
		Acao:        carga.Acao,
		RecebidoEm:  time.Now().UTC(),
	})
	if err != nil {
		return 0, err
	}

	entrega.diario.Info("pull request registrado",
		"task_run_id", execucao.ID, "acao", carga.Acao, "reentrega", execucao.JaRegistrada)

	return http.StatusAccepted, nil
}

func (s *Servidor) processarRevisao(ctx context.Context, entrega entregaRecebida) (int, error) {
	var carga cargaReview
	if err := json.Unmarshal(entrega.corpoBruto, &carga); err != nil {
		entrega.diario.Warn("corpo do webhook não é JSON válido")
		return http.StatusBadRequest, nil
	}

	if !slices.Contains(estadosDeReviewQueDecidem, carga.Review.Estado) {
		entrega.diario.Debug("revisão sem decisão ignorada", "estado", carga.Review.Estado)
		return http.StatusNoContent, nil
	}

	err := s.execucoes.RegistrarRevisao(ctx, domain.DecisaoDeReview{
		DeliveryID:  entrega.deliveryID,
		Repositorio: carga.Repositorio.NomeCompleto,
		NumeroPR:    carga.PullRequest.Numero,
		Estado:      carga.Review.Estado,
		Revisor:     carga.Review.Usuario.Login,
		RecebidoEm:  time.Now().UTC(),
	})
	if err != nil {
		return 0, err
	}

	entrega.diario.Info("decisão de revisão registrada",
		"numero_pr", carga.PullRequest.Numero, "estado", carga.Review.Estado)

	return http.StatusAccepted, nil
}
