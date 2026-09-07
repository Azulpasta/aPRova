package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const tempoLimiteVerificacao = 2 * time.Second

const (
	situacaoDisponivel   = "ok"
	situacaoIndisponivel = "indisponivel"
)

type respostaProntidao struct {
	Status       string            `json:"status"`
	Dependencias map[string]string `json:"dependencias"`
}

func (s *Servidor) manipularSaude(w http.ResponseWriter, _ *http.Request) {
	responderJSON(w, http.StatusOK, respostaSimples{Status: situacaoDisponivel})
}

func (s *Servidor) manipularProntidao(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := context.WithTimeout(r.Context(), tempoLimiteVerificacao)
	defer cancelar()

	falhas := make([]error, len(s.dependencias))
	var verificacoes sync.WaitGroup

	for indice, dependencia := range s.dependencias {
		verificacoes.Add(1)
		go func() {
			defer verificacoes.Done()
			falhas[indice] = dependencia.Verificar(ctx)
		}()
	}
	verificacoes.Wait()

	situacoes := make(map[string]string, len(s.dependencias))
	pronto := true

	for indice, dependencia := range s.dependencias {
		if err := falhas[indice]; err != nil {
			slog.Error("dependência indisponível", "dependencia", dependencia.Nome, "erro", err)
			situacoes[dependencia.Nome] = situacaoIndisponivel
			pronto = false
			continue
		}
		situacoes[dependencia.Nome] = situacaoDisponivel
	}

	if !pronto {
		responderJSON(w, http.StatusServiceUnavailable, respostaProntidao{
			Status:       situacaoIndisponivel,
			Dependencias: situacoes,
		})
		return
	}

	responderJSON(w, http.StatusOK, respostaProntidao{
		Status:       situacaoDisponivel,
		Dependencias: situacoes,
	})
}
