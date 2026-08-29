package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const tempoLimiteRequisicao = 30 * time.Second

// Dependencia identifica um serviço externo consultado pela rota de prontidão.
type Dependencia struct {
	Nome      string
	Verificar func(ctx context.Context) error
}

// Servidor agrupa as dependências externas usadas pelos handlers HTTP.
type Servidor struct {
	dependencias []Dependencia
}

// NovoServidor constrói o servidor HTTP com as dependências que serão
// verificadas pela rota de prontidão.
func NovoServidor(dependencias ...Dependencia) *Servidor {
	return &Servidor{dependencias: dependencias}
}

// Rotas devolve o roteador com os middlewares e as rotas registradas.
func (s *Servidor) Rotas() http.Handler {
	roteador := chi.NewRouter()

	roteador.Use(middleware.RequestID)
	roteador.Use(middleware.Logger)
	roteador.Use(middleware.Recoverer)
	roteador.Use(middleware.Timeout(tempoLimiteRequisicao))

	roteador.Get("/health", s.manipularSaude)
	roteador.Get("/ready", s.manipularProntidao)
	roteador.Post("/webhooks/github", s.manipularWebhookGitHub)

	return roteador
}
