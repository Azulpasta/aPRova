package repository

import (
	"context"
	"fmt"

	"github.com/Azulpasta/aPRova/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

const inserirTaskRun = `
WITH nova AS (
    INSERT INTO task_runs (delivery_id, repositorio, numero_pr, sha_head, autor, acao, recebido_em)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
    ON CONFLICT (delivery_id) DO NOTHING
    RETURNING id
)
SELECT id, true FROM nova
UNION ALL
SELECT id, false FROM task_runs
WHERE delivery_id = $1 AND NOT EXISTS (SELECT 1 FROM nova)
`

const inserirDecisaoDeReview = `
INSERT INTO decisoes_de_review (delivery_id, task_run_id, repositorio, numero_pr, estado, revisor, recebido_em)
VALUES (
    $1,
    (SELECT id FROM task_runs WHERE repositorio = $2 AND numero_pr = $3 ORDER BY criado_em DESC LIMIT 1),
    $2, $3, $4, $5, $6
)
ON CONFLICT (delivery_id) DO NOTHING
`

// Execucoes persiste as execuções e as decisões de revisão em PostgreSQL.
type Execucoes struct {
	pool *pgxpool.Pool
}

// NovoRegistroDeExecucoes constrói o repositório de execuções sobre o pool informado.
func NovoRegistroDeExecucoes(pool *pgxpool.Pool) *Execucoes {
	return &Execucoes{pool: pool}
}

// RegistrarPullRequest cria a execução no estado pendente. Uma reentrega do
// mesmo delivery ID devolve a execução já existente em vez de duplicá-la.
func (e *Execucoes) RegistrarPullRequest(ctx context.Context, evento domain.EventoPullRequest) (domain.Execucao, error) {
	var identificador string
	var criada bool

	err := e.pool.QueryRow(ctx, inserirTaskRun,
		evento.DeliveryID, evento.Repositorio, evento.NumeroPR,
		evento.SHAHead, evento.Autor, evento.Acao, evento.RecebidoEm,
	).Scan(&identificador, &criada)
	if err != nil {
		return domain.Execucao{}, fmt.Errorf("inserir task_run: %w", err)
	}

	return domain.Execucao{ID: identificador, JaRegistrada: !criada}, nil
}

// RegistrarRevisao grava a decisão da revisão, associando-a à execução mais
// recente do mesmo pull request quando ela existir.
func (e *Execucoes) RegistrarRevisao(ctx context.Context, decisao domain.DecisaoDeReview) error {
	_, err := e.pool.Exec(ctx, inserirDecisaoDeReview,
		decisao.DeliveryID, decisao.Repositorio, decisao.NumeroPR,
		decisao.Estado, decisao.Revisor, decisao.RecebidoEm,
	)
	if err != nil {
		return fmt.Errorf("inserir decisão de review: %w", err)
	}

	return nil
}
