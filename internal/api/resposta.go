package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type respostaSimples struct {
	Status string `json:"status"`
}

func responderJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(corpo); err != nil {
		slog.Error("falha ao escrever resposta JSON", "erro", err)
	}
}
