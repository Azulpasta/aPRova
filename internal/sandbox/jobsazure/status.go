package jobsazure

import "slices"

// Estados de execução de um job, como a Azure os reporta.
const (
	StatusEmProcessamento = "Processing"
	StatusRodando         = "Running"
	StatusConcluido       = "Succeeded"
	StatusFalhou          = "Failed"
	StatusParado          = "Stopped"
	StatusDegradado       = "Degraded"
	StatusDesconhecido    = "Unknown"
)

var statusDeFalha = []string{StatusFalhou, StatusParado, StatusDegradado}

// StatusDeFalha informa se a execução terminou sem sucesso.
func StatusDeFalha(status string) bool {
	return slices.Contains(statusDeFalha, status)
}

// StatusTerminal informa se a execução terminou, com ou sem sucesso.
func StatusTerminal(status string) bool {
	return status == StatusConcluido || StatusDeFalha(status)
}
