package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func registrarAcesso(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		resposta := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		proximo.ServeHTTP(resposta, r)

		slog.Info("requisição atendida",
			"metodo", r.Method,
			"rota", r.URL.Path,
			"status", statusEscrito(resposta),
			"duracao_ms", time.Since(inicio).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

func statusEscrito(resposta middleware.WrapResponseWriter) int {
	if resposta.Status() == 0 {
		return http.StatusOK
	}
	return resposta.Status()
}
