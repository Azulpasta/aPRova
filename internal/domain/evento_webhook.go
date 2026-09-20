package domain

import "time"

// EventoPullRequest descreve um evento de pull request aceito para processamento.
type EventoPullRequest struct {
	DeliveryID  string
	Repositorio string
	NumeroPR    int
	SHAHead     string
	Autor       string
	Acao        string
	RecebidoEm  time.Time
}

// DecisaoDeReview descreve a decisão de uma revisão humana sobre um pull request.
type DecisaoDeReview struct {
	DeliveryID  string
	Repositorio string
	NumeroPR    int
	Estado      string
	Revisor     string
	RecebidoEm  time.Time
}

// Execucao identifica o registro de execução associado a um evento recebido.
type Execucao struct {
	ID           string
	JaRegistrada bool
}
