package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/domain"
	"github.com/Azulpasta/aPRova/internal/repository/db"
	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

const (
	intervaloConsultaPadrao = 500 * time.Millisecond
	primeiraTentativa       = 1

	codigoViolacaoUnica = "23505"
)

var (
	_ domain.RepositorioTaskRun     = (*RepositorioPostgres)(nil)
	_ domain.RepositorioRecibo      = (*RepositorioPostgres)(nil)
	_ sandbox.ColetorResultado      = (*RepositorioPostgres)(nil)
	_ acaoremota.RegistroTentativas = (*RepositorioPostgres)(nil)
)

var validador = validator.New(validator.WithRequiredStructEnabled())

var decisaoPorEstadoDeReview = map[string]domain.DecisaoAprovacao{
	"approved":          domain.DecisaoAprovada,
	"changes_requested": domain.DecisaoAlteracoesSolicitadas,
}

// conexao é o que o repositório usa do pool ou de uma transação aberta.
type conexao interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// OpcoesPostgres ajusta o repositório. Intervalo vazio assume o padrão.
type OpcoesPostgres struct {
	// IntervaloConsulta é a espera entre consultas de Aguardar.
	IntervaloConsulta time.Duration
}

// RepositorioPostgres persiste o núcleo do aPRova em PostgreSQL. Implementa
// RepositorioTaskRun, RepositorioRecibo, sandbox.ColetorResultado e
// acaoremota.RegistroTentativas.
type RepositorioPostgres struct {
	conexao           conexao
	consultas         *db.Queries
	intervaloConsulta time.Duration
}

// NovoRepositorioPostgres constrói o repositório sobre o pool informado.
func NovoRepositorioPostgres(pool *pgxpool.Pool, opcoes OpcoesPostgres) *RepositorioPostgres {
	if opcoes.IntervaloConsulta <= 0 {
		opcoes.IntervaloConsulta = intervaloConsultaPadrao
	}
	return &RepositorioPostgres{conexao: pool, consultas: db.New(pool), intervaloConsulta: opcoes.IntervaloConsulta}
}

// EmTransacao roda fn com um repositório preso a uma transação. Erro ou panic
// em fn desfazem tudo; sem erro, a transação é confirmada. Chamadas
// aninhadas viram savepoints.
func (r *RepositorioPostgres) EmTransacao(ctx context.Context, fn func(tx *RepositorioPostgres) error) error {
	transacao, err := r.conexao.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação: %w", err)
	}
	defer func() {
		if err := transacao.Rollback(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.ErrorContext(ctx, "falha ao desfazer transação", "erro", err)
		}
	}()

	if err := fn(&RepositorioPostgres{conexao: transacao, consultas: r.consultas.WithTx(transacao), intervaloConsulta: r.intervaloConsulta}); err != nil {
		return err
	}

	if err := transacao.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar transação: %w", err)
	}
	return nil
}

// CriarTaskRun cria o task_run no estado pendente. Uma reentrega do mesmo
// delivery_id devolve o existente com criado igual a false.
func (r *RepositorioPostgres) CriarTaskRun(ctx context.Context, novo domain.NovoTaskRun) (domain.TaskRun, bool, error) {
	if err := validar(novo); err != nil {
		return domain.TaskRun{}, false, err
	}

	linha, err := r.consultas.CreateTaskRun(ctx, db.CreateTaskRunParams{
		TenantID:    novo.TenantID,
		DeliveryID:  novo.DeliveryID,
		Repository:  novo.Repositorio,
		PrNumber:    int32(novo.NumeroPR), //nolint:gosec // validado entre 1 e math.MaxInt32
		HeadSha:     novo.SHAHead,
		Author:      novo.Autor,
		EventAction: novo.Acao,
		ReceivedAt:  novo.RecebidoEm,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existente, err := r.BuscarTaskRunPorDeliveryID(ctx, novo.DeliveryID)
		return existente, false, err
	}
	if err != nil {
		return domain.TaskRun{}, false, traduzir("inserir task_run", err)
	}

	return paraTaskRun(linha), true, nil
}

// BuscarTaskRun devolve o task_run pelo id.
func (r *RepositorioPostgres) BuscarTaskRun(ctx context.Context, id string) (domain.TaskRun, error) {
	linha, err := r.consultas.GetTaskRun(ctx, id)
	if err != nil {
		return domain.TaskRun{}, traduzir("buscar task_run", err)
	}
	return paraTaskRun(linha), nil
}

// BuscarTaskRunPorDeliveryID devolve o task_run criado pela entrega informada.
func (r *RepositorioPostgres) BuscarTaskRunPorDeliveryID(ctx context.Context, deliveryID string) (domain.TaskRun, error) {
	linha, err := r.consultas.GetTaskRunByDeliveryID(ctx, deliveryID)
	if err != nil {
		return domain.TaskRun{}, traduzir("buscar task_run por delivery_id", err)
	}
	return paraTaskRun(linha), nil
}

// TransicionarTaskRun move o task_run de De para Para e registra a etapa, na
// mesma transação. Se o task_run não estiver mais em De, devolve
// ErrTransicaoConcorrente sem mudar nada.
func (r *RepositorioPostgres) TransicionarTaskRun(ctx context.Context, transicao domain.Transicao) (domain.TaskRun, error) {
	if err := validar(transicao); err != nil {
		return domain.TaskRun{}, err
	}

	var atualizado domain.TaskRun
	err := r.EmTransacao(ctx, func(tx *RepositorioPostgres) error {
		linha, err := tx.consultas.TransitionTaskRun(ctx, db.TransitionTaskRunParams{
			ID: transicao.TaskRunID, FromState: string(transicao.De), ToState: string(transicao.Para),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.semLinhaNaTransicao(ctx, transicao)
		}
		if err != nil {
			return traduzir("transicionar task_run", err)
		}

		_, err = tx.consultas.CreateTaskStep(ctx, db.CreateTaskStepParams{
			TaskRunID: transicao.TaskRunID, FromState: string(transicao.De), ToState: string(transicao.Para), Detail: transicao.Detalhe,
		})
		if err != nil {
			return traduzir("registrar etapa", err)
		}

		atualizado = paraTaskRun(linha)
		return nil
	})

	return atualizado, err
}

// RegistrarPullRequest cria o task_run de um webhook de pull request.
func (r *RepositorioPostgres) RegistrarPullRequest(ctx context.Context, evento domain.EventoPullRequest) (domain.Execucao, error) {
	taskRun, criado, err := r.CriarTaskRun(ctx, domain.NovoTaskRun{
		TenantID:    evento.InstallationID,
		DeliveryID:  evento.DeliveryID,
		Repositorio: evento.Repositorio,
		NumeroPR:    evento.NumeroPR,
		SHAHead:     evento.SHAHead,
		Autor:       evento.Autor,
		Acao:        evento.Acao,
		RecebidoEm:  evento.RecebidoEm,
	})
	if err != nil {
		return domain.Execucao{}, err
	}
	return domain.Execucao{ID: taskRun.ID, JaRegistrada: !criado}, nil
}

// RegistrarRevisao grava a decisão de revisão no task_run mais recente do
// pull request. Sem task_run, devolve ErrNaoEncontrado. Uma reentrega do
// mesmo delivery_id não duplica a aprovação.
func (r *RepositorioPostgres) RegistrarRevisao(ctx context.Context, decisao domain.DecisaoDeReview) error {
	decisaoAprovacao, conhecida := decisaoPorEstadoDeReview[decisao.Estado]
	if !conhecida {
		return fmt.Errorf("%w: estado de review %q", domain.ErrDadosInvalidos, decisao.Estado)
	}
	numeroPR, err := numeroDePR(decisao.NumeroPR)
	if err != nil {
		return err
	}

	taskRun, err := r.consultas.GetLatestTaskRunForPullRequest(ctx, db.GetLatestTaskRunForPullRequestParams{
		TenantID:   decisao.InstallationID,
		Repository: decisao.Repositorio,
		PrNumber:   numeroPR,
	})
	if err != nil {
		return traduzir("buscar task_run do pull request", err)
	}

	return r.RegistrarAprovacao(ctx, domain.Aprovacao{
		TaskRunID:  taskRun.ID,
		DeliveryID: decisao.DeliveryID,
		Decisao:    decisaoAprovacao,
		Revisor:    decisao.Revisor,
		DecididaEm: decisao.RecebidoEm,
	})
}

// RegistrarAprovacao grava a decisão humana sobre um task_run.
func (r *RepositorioPostgres) RegistrarAprovacao(ctx context.Context, aprovacao domain.Aprovacao) error {
	if err := validar(aprovacao); err != nil {
		return err
	}

	var motivo *string
	if aprovacao.MotivoFeedback != nil {
		texto := string(*aprovacao.MotivoFeedback)
		motivo = &texto
	}

	inseridas, err := r.consultas.CreateApproval(ctx, db.CreateApprovalParams{
		TaskRunID:      aprovacao.TaskRunID,
		DeliveryID:     aprovacao.DeliveryID,
		Decision:       string(aprovacao.Decisao),
		Reviewer:       aprovacao.Revisor,
		FeedbackReason: motivo,
		Justification:  aprovacao.Justificativa,
		DecidedAt:      aprovacao.DecididaEm,
	})
	if err != nil {
		return traduzir("registrar aprovação", err)
	}
	if inseridas > 0 {
		return nil
	}

	// Nenhuma linha: ou é reentrega do mesmo delivery_id, que não é erro, ou o
	// task_run não existe.
	existe, err := r.consultas.TaskRunExists(ctx, aprovacao.TaskRunID)
	if err != nil {
		return traduzir("conferir task_run da aprovação", err)
	}
	if !existe {
		return fmt.Errorf("registrar aprovação: task_run %s: %w", aprovacao.TaskRunID, domain.ErrNaoEncontrado)
	}
	return nil
}

// GravarGateDeRisco grava a decisão do gate. Só existe uma por task_run.
func (r *RepositorioPostgres) GravarGateDeRisco(ctx context.Context, gate domain.GateDeRisco) error {
	if err := validar(gate); err != nil {
		return err
	}

	_, err := r.consultas.CreateRiskGate(ctx, db.CreateRiskGateParams{
		TaskRunID:          gate.TaskRunID,
		DeterministicLevel: string(gate.NivelDeterministico),
		Level:              string(gate.Nivel),
		Reason:             gate.Motivo,
	})
	return traduzir("gravar gate de risco", err)
}

// AnexarRecibo grava o recibo. Só existe um por task_run e ele nunca muda.
func (r *RepositorioPostgres) AnexarRecibo(ctx context.Context, novo domain.NovoRecibo) (domain.Recibo, error) {
	if err := validar(novo); err != nil {
		return domain.Recibo{}, err
	}

	linha, err := r.consultas.AppendReceipt(ctx, db.AppendReceiptParams{
		TaskRunID:         novo.TaskRunID,
		PreviousReceiptID: novo.ReciboAnteriorID,
		Payload:           novo.Conteudo,
		ContentHash:       novo.Hash,
		Signature:         novo.Assinatura,
		KeyID:             novo.IDChave,
		SignedAt:          novo.AssinadoEm,
	})
	if err != nil {
		return domain.Recibo{}, traduzir("anexar recibo", err)
	}
	return paraRecibo(linha), nil
}

// BuscarRecibo devolve o recibo pelo id.
func (r *RepositorioPostgres) BuscarRecibo(ctx context.Context, id string) (domain.Recibo, error) {
	linha, err := r.consultas.GetReceipt(ctx, id)
	if err != nil {
		return domain.Recibo{}, traduzir("buscar recibo", err)
	}
	return paraRecibo(linha), nil
}

// BuscarReciboPorTaskRun devolve o recibo do task_run.
func (r *RepositorioPostgres) BuscarReciboPorTaskRun(ctx context.Context, taskRunID string) (domain.Recibo, error) {
	linha, err := r.consultas.GetReceiptByTaskRun(ctx, taskRunID)
	if err != nil {
		return domain.Recibo{}, traduzir("buscar recibo por task_run", err)
	}
	return paraRecibo(linha), nil
}

// Gravar registra o resultado do sandbox para o task_run. Uma segunda
// gravação é recusada com sandbox.ErrResultadoJaGravado, como no coletor em
// memória.
func (r *RepositorioPostgres) Gravar(ctx context.Context, taskRunID string, resultado sandbox.ResultadoSandbox) error {
	_, err := r.consultas.CreateSandboxResult(ctx, db.CreateSandboxResultParams{
		TaskRunID:   taskRunID,
		Attempt:     primeiraTentativa,
		Compiled:    resultado.Compilou,
		TestsPassed: int32(resultado.TestesPassaram), //nolint:gosec // contagem de testes cabe em int32
		TestsFailed: int32(resultado.TestesFalharam), //nolint:gosec // contagem de testes cabe em int32
		RawOutput:   resultado.SaidaBruta,
		DurationMs:  resultado.DuracaoMS,
		FailureType: string(resultado.TipoFalha),
	})
	if errors.Is(traduzir("", err), domain.ErrDuplicado) {
		return fmt.Errorf("%w: %s", sandbox.ErrResultadoJaGravado, taskRunID)
	}
	return traduzir("gravar resultado do sandbox", err)
}

// Aguardar consulta o banco até o resultado do task_run aparecer ou o
// contexto terminar, o que vier primeiro.
func (r *RepositorioPostgres) Aguardar(ctx context.Context, taskRunID string) (sandbox.ResultadoSandbox, error) {
	for {
		linha, err := r.consultas.GetLatestSandboxResult(ctx, taskRunID)
		if err == nil {
			return paraResultadoSandbox(linha), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			if ctx.Err() != nil {
				return sandbox.ResultadoSandbox{}, ctx.Err()
			}
			return sandbox.ResultadoSandbox{}, traduzir("ler resultado do sandbox", err)
		}

		select {
		case <-ctx.Done():
			return sandbox.ResultadoSandbox{}, ctx.Err()
		case <-time.After(r.intervaloConsulta):
		}
	}
}

// GravarMemoriaDeCodigo guarda uma decisão passada para a calibração.
func (r *RepositorioPostgres) GravarMemoriaDeCodigo(ctx context.Context, memoria domain.MemoriaDeCodigo) (int64, error) {
	if err := validar(memoria); err != nil {
		return 0, err
	}

	id, err := r.consultas.CreateCodeMemory(ctx, db.CreateCodeMemoryParams{
		TenantID:   memoria.TenantID,
		TaskRunID:  memoria.TaskRunID,
		Repository: memoria.Repositorio,
		Summary:    memoria.Resumo,
		Embedding:  pgvector.NewVector(memoria.Embedding),
		Level:      string(memoria.Nivel),
		Outcome:    string(memoria.Desfecho),
	})
	if err != nil {
		return 0, traduzir("gravar memória de código: task_run inexistente ou de outro tenant", err)
	}
	return id, nil
}

// BuscarVizinhos devolve as memórias do tenant mais próximas do embedding,
// da mais próxima para a mais distante, por distância de cosseno.
func (r *RepositorioPostgres) BuscarVizinhos(ctx context.Context, tenantID int64, embedding []float32, limite int) ([]domain.Vizinho, error) {
	if len(embedding) != domain.DimensoesEmbedding {
		return nil, fmt.Errorf("%w: embedding com %d dimensões, esperava %d", domain.ErrDadosInvalidos, len(embedding), domain.DimensoesEmbedding)
	}
	if limite <= 0 || limite > 1000 {
		return nil, fmt.Errorf("%w: limite %d fora de 1..1000", domain.ErrDadosInvalidos, limite)
	}

	var linhas []db.FindNearestCodeMemoryRow
	err := r.EmTransacao(ctx, func(tx *RepositorioPostgres) error {
		// Sem a varredura iterativa, o HNSW entrega só hnsw.ef_search candidatos
		// de todos os tenants e o filtro por tenant pode deixar o resultado vazio.
		if _, err := tx.conexao.Exec(ctx, "SELECT set_config('hnsw.iterative_scan', 'strict_order', true)"); err != nil {
			return fmt.Errorf("ligar varredura iterativa do hnsw: %w", err)
		}

		var err error
		linhas, err = tx.consultas.FindNearestCodeMemory(ctx, db.FindNearestCodeMemoryParams{
			Embedding:  pgvector.NewVector(embedding),
			TenantID:   tenantID,
			MaxResults: int32(limite), //nolint:gosec // limitado a 1000 acima
		})
		return traduzir("buscar vizinhos", err)
	})
	if err != nil {
		return nil, err
	}

	vizinhos := make([]domain.Vizinho, 0, len(linhas))
	for _, linha := range linhas {
		vizinhos = append(vizinhos, domain.Vizinho{
			ID:          linha.ID,
			TaskRunID:   linha.TaskRunID,
			Repositorio: linha.Repository,
			Resumo:      linha.Summary,
			Nivel:       domain.NivelRisco(linha.Level),
			Desfecho:    domain.DesfechoMudanca(linha.Outcome),
			Distancia:   linha.Distance,
		})
	}
	return vizinhos, nil
}

// VincularIdentidade associa o usuário do Slack ao login do GitHub no tenant,
// substituindo um vínculo anterior.
func (r *RepositorioPostgres) VincularIdentidade(ctx context.Context, tenantID int64, usuarioSlack, loginGitHub string) error {
	_, err := r.consultas.UpsertIdentityLink(ctx, db.UpsertIdentityLinkParams{
		TenantID: tenantID, SlackUserID: usuarioSlack, GithubLogin: loginGitHub,
	})
	return traduzir("vincular identidade", err)
}

// ResolverIdentidade devolve o login do GitHub vinculado ao usuário do Slack
// no tenant, ou ErrNaoEncontrado.
func (r *RepositorioPostgres) ResolverIdentidade(ctx context.Context, tenantID int64, usuarioSlack string) (string, error) {
	login, err := r.consultas.ResolveIdentityLink(ctx, db.ResolveIdentityLinkParams{TenantID: tenantID, SlackUserID: usuarioSlack})
	if err != nil {
		return "", traduzir("resolver identidade", err)
	}
	return login, nil
}

// Registrar guarda a tentativa de ação remota. Pedidos inválidos ficam só no
// log, porque não identificam com segurança tenant, tipo nem pull request. Um
// clique repetido com o mesmo id_acao mantém o primeiro registro, e a
// repetição também fica só no log.
func (r *RepositorioPostgres) Registrar(ctx context.Context, tentativa acaoremota.Tentativa) error {
	if tentativa.Decisao == acaoremota.DecisaoInvalida || tentativa.Decisao == acaoremota.DecisaoRepetida {
		return nil
	}
	numeroPR, err := numeroDePR(tentativa.NumeroPR)
	if err != nil {
		return err
	}

	var login *string
	if tentativa.LoginGitHub != "" {
		login = &tentativa.LoginGitHub
	}

	_, err = r.consultas.CreateRemoteActionAttempt(ctx, db.CreateRemoteActionAttemptParams{
		ActionID:    tentativa.IDAcao,
		TenantID:    tentativa.InstallationID,
		Repository:  tentativa.Repositorio,
		PrNumber:    numeroPR,
		ActionType:  string(tentativa.Tipo),
		SlackUserID: tentativa.UsuarioSlack,
		GithubLogin: login,
		Decision:    string(tentativa.Decisao),
		Reason:      string(tentativa.Motivo),
		AttemptedAt: tentativa.Em,
	})
	return traduzir("registrar tentativa de ação remota", err)
}

// semLinhaNaTransicao distingue o task_run que mudou de estado do que nunca
// existiu.
func (r *RepositorioPostgres) semLinhaNaTransicao(ctx context.Context, transicao domain.Transicao) error {
	existe, err := r.consultas.TaskRunExists(ctx, transicao.TaskRunID)
	if err != nil {
		return traduzir("conferir task_run da transição", err)
	}
	if !existe {
		return fmt.Errorf("transicionar task_run %s: %w", transicao.TaskRunID, domain.ErrNaoEncontrado)
	}
	return fmt.Errorf("%w: %s não está em %s", domain.ErrTransicaoConcorrente, transicao.TaskRunID, transicao.De)
}

// numeroDePR converte o número do pull request para a coluna integer, que
// não comporta todo int.
func numeroDePR(numero int) (int32, error) {
	if numero <= 0 || numero > math.MaxInt32 {
		return 0, fmt.Errorf("%w: número de PR %d fora de 1..%d", domain.ErrDadosInvalidos, numero, math.MaxInt32)
	}
	return int32(numero), nil
}

func validar(valor any) error {
	if err := validador.Struct(valor); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrDadosInvalidos, err)
	}
	return nil
}

// traduzir converte erros do driver nos erros do domínio, mantendo a causa.
func traduzir(etapa string, err error) error {
	if err == nil {
		return nil
	}

	var erroPg *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%s: %w", etapa, domain.ErrNaoEncontrado)
	case errors.As(err, &erroPg) && erroPg.Code == codigoViolacaoUnica:
		return fmt.Errorf("%s: %w: %w", etapa, domain.ErrDuplicado, err)
	default:
		return fmt.Errorf("%s: %w", etapa, err)
	}
}

func paraTaskRun(linha db.TaskRun) domain.TaskRun {
	return domain.TaskRun{
		ID:           linha.ID,
		TenantID:     linha.TenantID,
		DeliveryID:   linha.DeliveryID,
		Repositorio:  linha.Repository,
		NumeroPR:     int(linha.PrNumber),
		SHAHead:      linha.HeadSha,
		Autor:        linha.Author,
		Acao:         linha.EventAction,
		Estado:       domain.EstadoTaskRun(linha.State),
		RecebidoEm:   linha.ReceivedAt,
		CriadoEm:     linha.CreatedAt,
		AtualizadoEm: linha.UpdatedAt,
	}
}

func paraRecibo(linha db.Receipt) domain.Recibo {
	return domain.Recibo{
		NovoRecibo: domain.NovoRecibo{
			TaskRunID:        linha.TaskRunID,
			ReciboAnteriorID: linha.PreviousReceiptID,
			Conteudo:         linha.Payload,
			Hash:             linha.ContentHash,
			Assinatura:       linha.Signature,
			IDChave:          linha.KeyID,
			AssinadoEm:       linha.SignedAt,
		},
		ID:       linha.ID,
		TenantID: linha.TenantID,
		CriadoEm: linha.CreatedAt,
	}
}

func paraResultadoSandbox(linha db.SandboxResult) sandbox.ResultadoSandbox {
	return sandbox.ResultadoSandbox{
		Compilou:       linha.Compiled,
		TestesPassaram: int(linha.TestsPassed),
		TestesFalharam: int(linha.TestsFailed),
		SaidaBruta:     linha.RawOutput,
		DuracaoMS:      linha.DurationMs,
		TipoFalha:      sandbox.TipoFalha(linha.FailureType),
	}
}
