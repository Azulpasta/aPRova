package platform

// EventoReview é o veredito de uma review, como a API do GitHub o nomeia.
type EventoReview string

// Vereditos de review usados pelo aPRova.
const (
	EventoAprovar            EventoReview = "APPROVE"
	EventoSolicitarAlteracao EventoReview = "REQUEST_CHANGES"
)

// Review é o conteúdo de uma review a publicar num pull request.
type Review struct {
	Evento EventoReview
	Corpo  string
}
