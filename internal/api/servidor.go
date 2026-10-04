package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const tempoLimiteRequisicao = 30 * time.Second

// Dependencia identifica um serviço externo consultado pela rota de prontidão.
type Dependencia struct {
	Nome      string
	Verificar func(ctx context.Context) error
}

// RegistroDeExecucoes descreve a persistência exigida pelo handler de webhook.
type RegistroDeExecucoes interface {
	RegistrarPullRequest(ctx context.Context, evento domain.EventoPullRequest) (domain.Execucao, error)
	RegistrarRevisao(ctx context.Context, decisao domain.DecisaoDeReview) error
}

// ProcessadorDeAcoesRemotas decide e executa uma ação pedida por canal
// externo, a partir do corpo já autenticado.
type ProcessadorDeAcoesRemotas interface {
	Processar(ctx context.Context, corpo []byte) acaoremota.Decisao
}

// Opcoes reúne as dependências com que o servidor HTTP é construído.
type Opcoes struct {
	Dependencias        []Dependencia
	SegredoWebhook      []byte
	Execucoes           RegistroDeExecucoes
	SegredoAcoesRemotas []byte
	AcoesRemotas        ProcessadorDeAcoesRemotas
	Agora               func() time.Time
}

// Servidor agrupa as dependências externas usadas pelos handlers HTTP.
type Servidor struct {
	dependencias        []Dependencia
	segredoWebhook      []byte
	execucoes           RegistroDeExecucoes
	segredoAcoesRemotas []byte
	acoesRemotas        ProcessadorDeAcoesRemotas
	agora               func() time.Time
}

// NovoServidor constrói o servidor HTTP a partir das opções informadas.
func NovoServidor(opcoes Opcoes) *Servidor {
	agora := opcoes.Agora
	if agora == nil {
		agora = time.Now
	}

	return &Servidor{
		dependencias:        opcoes.Dependencias,
		segredoWebhook:      opcoes.SegredoWebhook,
		execucoes:           opcoes.Execucoes,
		segredoAcoesRemotas: opcoes.SegredoAcoesRemotas,
		acoesRemotas:        opcoes.AcoesRemotas,
		agora:               agora,
	}
}

// Rotas devolve o roteador com os middlewares e as rotas registradas.
func (s *Servidor) Rotas() http.Handler {
	roteador := chi.NewRouter()

	roteador.Use(middleware.RequestID)
	roteador.Use(registrarAcesso)
	roteador.Use(middleware.Recoverer)
	roteador.Use(middleware.Timeout(tempoLimiteRequisicao))

	roteador.Get("/health", s.manipularSaude)
	roteador.Get("/ready", s.manipularProntidao)
	roteador.Post("/webhooks/github", s.manipularWebhookGitHub)
	roteador.Post("/acoes-remotas", s.manipularAcaoRemota)

	return roteador
}
