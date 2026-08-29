package api

import "net/http"

func (s *Servidor) manipularWebhookGitHub(w http.ResponseWriter, _ *http.Request) {
	responderJSON(w, http.StatusAccepted, respostaSimples{Status: "aceito"})
}
