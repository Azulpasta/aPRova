package integracaopostgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/domain"
	"github.com/Azulpasta/aPRova/internal/repository"
	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errProposital = errors.New("erro proposital")

func TestDeliveryIDRepetidoFalhaNoBanco(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())

	_, err := pool.Exec(t.Context(), `
		INSERT INTO task_runs (tenant_id, delivery_id, repository, pr_number, head_sha, author, event_action, received_at)
		VALUES (1, $1, 'outro/repo', 1, 'abc', 'outro', 'opened', now())`, taskRun.DeliveryID)

	if codigoPostgres(err) != "23505" {
		t.Fatalf("esperava unique_violation (23505) para delivery_id repetido, obtive %v", err)
	}
}

func TestCriarTaskRunNaReentregaDevolveOExistente(t *testing.T) {
	repo := exigirBanco(t)
	novo := domain.NovoTaskRun{
		TenantID: novoTenant(), DeliveryID: "entrega-reentregue-" + time.Now().Format(time.RFC3339Nano),
		Repositorio: "Azulpasta/alvo", NumeroPR: 3, SHAHead: "0123456789abcdef0123456789abcdef01234567",
		Autor: "autora", Acao: "opened", RecebidoEm: time.Now().UTC(),
	}

	primeiro, criado, err := repo.CriarTaskRun(t.Context(), novo)
	if err != nil || !criado {
		t.Fatalf("primeira entrega: criado=%v err=%v", criado, err)
	}
	segundo, criado, err := repo.CriarTaskRun(t.Context(), novo)
	if err != nil {
		t.Fatalf("reentrega: %v", err)
	}
	if criado || segundo.ID != primeiro.ID {
		t.Errorf("a reentrega deveria devolver o task_run existente %s sem criar outro, obtive %s criado=%v", primeiro.ID, segundo.ID, criado)
	}
	if primeiro.Estado != domain.EstadoPendente {
		t.Errorf("task_run novo deveria começar pendente, está %s", primeiro.Estado)
	}
}

func TestCriarTaskRunValidaOsCampos(t *testing.T) {
	repo := exigirBanco(t)

	_, _, err := repo.CriarTaskRun(t.Context(), domain.NovoTaskRun{DeliveryID: "sem-tenant"})
	if !errors.Is(err, domain.ErrDadosInvalidos) {
		t.Errorf("esperava ErrDadosInvalidos, obtive %v", err)
	}
}

func TestBuscarTaskRunPorDeliveryID(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())

	encontrado, err := repo.BuscarTaskRunPorDeliveryID(t.Context(), taskRun.DeliveryID)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if encontrado.ID != taskRun.ID || encontrado.TenantID != taskRun.TenantID {
		t.Errorf("esperava %+v, obtive %+v", taskRun, encontrado)
	}

	if _, err := repo.BuscarTaskRunPorDeliveryID(t.Context(), "nao-existe"); !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func TestTransicionarRegistraEtapaERecusaEstadoDesatualizado(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())

	transicao := domain.Transicao{TaskRunID: taskRun.ID, De: domain.EstadoPendente, Para: domain.EstadoMapeandoContexto, Detalhe: "worker 1"}
	atualizado, err := repo.TransicionarTaskRun(t.Context(), transicao)
	if err != nil {
		t.Fatalf("transicionar: %v", err)
	}
	if atualizado.Estado != domain.EstadoMapeandoContexto {
		t.Errorf("esperava mapeando_contexto, obtive %s", atualizado.Estado)
	}

	if _, err := repo.TransicionarTaskRun(t.Context(), transicao); !errors.Is(err, domain.ErrTransicaoConcorrente) {
		t.Errorf("a segunda transição a partir de pendente deveria falhar com ErrTransicaoConcorrente, obtive %v", err)
	}

	if etapas := contar(t, `SELECT count(*) FROM task_steps WHERE task_run_id = $1`, taskRun.ID); etapas != 1 {
		t.Errorf("esperava 1 etapa registrada, obtive %d", etapas)
	}
}

func TestTransicionarTaskRunInexistenteDevolveNaoEncontrado(t *testing.T) {
	repo := exigirBanco(t)

	_, err := repo.TransicionarTaskRun(t.Context(), domain.Transicao{
		TaskRunID: "00000000-0000-0000-0000-000000000000", De: domain.EstadoPendente, Para: domain.EstadoMapeandoContexto,
	})
	if !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func TestTransicionarPermiteOLacoDeRevisaoSemDecisao(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())
	if _, err := pool.Exec(t.Context(), `UPDATE task_runs SET state = 'aguardando_aprovacao' WHERE id = $1`, taskRun.ID); err != nil {
		t.Fatalf("preparar estado: %v", err)
	}

	_, err := repo.TransicionarTaskRun(t.Context(), domain.Transicao{
		TaskRunID: taskRun.ID, De: domain.EstadoAguardandoAprovacao, Para: domain.EstadoAguardandoAprovacao, Detalhe: "review sem decisão",
	})
	if err != nil {
		t.Errorf("aguardando_aprovacao → aguardando_aprovacao está em docs/maquina-estados.mmd e deveria passar: %v", err)
	}
}

func TestCriarTaskRunRecusaNumeroDePRForaDoInteger(t *testing.T) {
	repo := exigirBanco(t)

	_, _, err := repo.CriarTaskRun(t.Context(), domain.NovoTaskRun{
		TenantID: novoTenant(), DeliveryID: "pr-gigante", Repositorio: "Azulpasta/alvo", NumeroPR: 1 << 31,
		SHAHead: "0123456789abcdef0123456789abcdef01234567", Autor: "a", Acao: "opened", RecebidoEm: time.Now(),
	})
	if !errors.Is(err, domain.ErrDadosInvalidos) {
		t.Errorf("esperava ErrDadosInvalidos para PR acima de int32, obtive %v", err)
	}
}

func TestTransacaoDesfeitaNaoDeixaLinha(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()

	err := repo.EmTransacao(t.Context(), func(tx *repository.RepositorioPostgres) error {
		taskRun := novoTaskRun(t, tx, tenant)
		if _, err := tx.TransicionarTaskRun(t.Context(), domain.Transicao{
			TaskRunID: taskRun.ID, De: domain.EstadoPendente, Para: domain.EstadoMapeandoContexto,
		}); err != nil {
			return err
		}
		return errProposital
	})

	if !errors.Is(err, errProposital) {
		t.Fatalf("EmTransacao deveria devolver o erro da função, obtive %v", err)
	}
	if sobrou := contar(t, `SELECT count(*) FROM task_runs WHERE tenant_id = $1`, tenant); sobrou != 0 {
		t.Errorf("a transação desfeita deixou %d task_runs", sobrou)
	}
	if sobrou := contar(t, `SELECT count(*) FROM task_steps WHERE tenant_id = $1`, tenant); sobrou != 0 {
		t.Errorf("a transação desfeita deixou %d task_steps", sobrou)
	}
}

func TestTransacaoConfirmadaGravaTudo(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()

	err := repo.EmTransacao(t.Context(), func(tx *repository.RepositorioPostgres) error {
		novoTaskRun(t, tx, tenant)
		return nil
	})
	if err != nil {
		t.Fatalf("EmTransacao: %v", err)
	}
	if total := contar(t, `SELECT count(*) FROM task_runs WHERE tenant_id = $1`, tenant); total != 1 {
		t.Errorf("esperava 1 task_run confirmado, obtive %d", total)
	}
}

func reciboDeExemplo(taskRunID string) domain.NovoRecibo {
	hash := sha256.Sum256([]byte("conteudo"))
	return domain.NovoRecibo{
		TaskRunID:  taskRunID,
		Conteudo:   `{"task_run_id":"` + taskRunID + `"}`,
		Hash:       hash[:],
		Assinatura: make([]byte, 64),
		IDChave:    "chave-1",
		AssinadoEm: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestReciboAnexadoPodeSerLido(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())

	novo := reciboDeExemplo(taskRun.ID)
	anexado, err := repo.AnexarRecibo(t.Context(), novo)
	if err != nil {
		t.Fatalf("anexar: %v", err)
	}

	lido, err := repo.BuscarRecibo(t.Context(), anexado.ID)
	if err != nil {
		t.Fatalf("buscar: %v", err)
	}
	if lido.Conteudo != anexado.Conteudo || string(lido.Hash) != string(anexado.Hash) || lido.TenantID != taskRun.TenantID {
		t.Errorf("recibo lido difere do anexado: %+v x %+v", lido, anexado)
	}
	if !lido.AssinadoEm.Equal(novo.AssinadoEm) {
		t.Errorf("assinado_em mudou: %v", lido.AssinadoEm)
	}

	porTaskRun, err := repo.BuscarReciboPorTaskRun(t.Context(), taskRun.ID)
	if err != nil || porTaskRun.ID != anexado.ID {
		t.Errorf("buscar por task_run: %+v %v", porTaskRun, err)
	}

	if _, err := repo.AnexarRecibo(t.Context(), reciboDeExemplo(taskRun.ID)); !errors.Is(err, domain.ErrDuplicado) {
		t.Errorf("um segundo recibo para o mesmo task_run deveria dar ErrDuplicado, obtive %v", err)
	}
}

func TestReciboSoEncadeiaComReciboDoMesmoTenant(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()
	original, err := repo.AnexarRecibo(t.Context(), reciboDeExemplo(novoTaskRun(t, repo, tenant).ID))
	if err != nil {
		t.Fatalf("anexar original: %v", err)
	}

	reversao := reciboDeExemplo(novoTaskRun(t, repo, tenant).ID)
	reversao.ReciboAnteriorID = &original.ID
	if _, err := repo.AnexarRecibo(t.Context(), reversao); err != nil {
		t.Errorf("encadear no mesmo tenant deveria funcionar: %v", err)
	}

	deOutroTenant := reciboDeExemplo(novoTaskRun(t, repo, novoTenant()).ID)
	deOutroTenant.ReciboAnteriorID = &original.ID
	if _, err := repo.AnexarRecibo(t.Context(), deOutroTenant); !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("encadear em recibo de outro tenant deveria dar ErrNaoEncontrado, obtive %v", err)
	}
}

func TestReceiptsRecusaUpdateDeleteETruncateNoBanco(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())
	recibo, err := repo.AnexarRecibo(t.Context(), reciboDeExemplo(taskRun.ID))
	if err != nil {
		t.Fatalf("anexar: %v", err)
	}

	comandos := map[string]string{
		"UPDATE":   `UPDATE receipts SET payload = 'adulterado' WHERE id = $1`,
		"DELETE":   `DELETE FROM receipts WHERE id = $1`,
		"TRUNCATE": `TRUNCATE receipts CASCADE`,
	}
	for operacao, comando := range comandos {
		t.Run(operacao, func(t *testing.T) {
			var err error
			if operacao == "TRUNCATE" {
				_, err = pool.Exec(t.Context(), comando)
			} else {
				_, err = pool.Exec(t.Context(), comando, recibo.ID)
			}
			if codigoPostgres(err) != "23001" {
				t.Errorf("o banco deveria recusar %s em receipts com restrict_violation (23001), obtive %v", operacao, err)
			}
		})
	}

	lido, err := repo.BuscarRecibo(t.Context(), recibo.ID)
	if err != nil || lido.Conteudo != recibo.Conteudo {
		t.Errorf("o recibo deveria continuar intacto: %+v %v", lido, err)
	}
}

func TestApagarTaskRunComReciboFalha(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())
	if _, err := repo.AnexarRecibo(t.Context(), reciboDeExemplo(taskRun.ID)); err != nil {
		t.Fatalf("anexar: %v", err)
	}

	_, err := pool.Exec(t.Context(), `DELETE FROM task_runs WHERE id = $1`, taskRun.ID)
	if codigoPostgres(err) != "23503" {
		t.Fatalf("apagar task_run com recibo deveria falhar com foreign_key_violation (23503), obtive %v", err)
	}
}

func TestOnDeleteDasFilhasSemRestricao(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()
	taskRun := novoTaskRun(t, repo, tenant)

	if _, err := repo.TransicionarTaskRun(t.Context(), domain.Transicao{TaskRunID: taskRun.ID, De: domain.EstadoPendente, Para: domain.EstadoMapeandoContexto}); err != nil {
		t.Fatalf("transicionar: %v", err)
	}
	if err := repo.Gravar(t.Context(), taskRun.ID, sandbox.ResultadoSandbox{Compilou: true, TipoFalha: sandbox.FalhaNenhuma}); err != nil {
		t.Fatalf("gravar resultado: %v", err)
	}
	if _, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(tenant, &taskRun.ID, vetorUnitario(0))); err != nil {
		t.Fatalf("gravar memória: %v", err)
	}
	if err := repo.Registrar(t.Context(), tentativaDeExemplo(tenant, "acao-on-delete-"+taskRun.ID)); err != nil {
		t.Fatalf("registrar tentativa: %v", err)
	}

	if _, err := pool.Exec(t.Context(), `DELETE FROM task_runs WHERE id = $1`, taskRun.ID); err != nil {
		t.Fatalf("apagar task_run sem gate, recibo nem aprovação deveria funcionar: %v", err)
	}

	for tabela, esperado := range map[string]int{"task_steps": 0, "sandbox_results": 0} {
		if total := contar(t, `SELECT count(*) FROM `+tabela+` WHERE tenant_id = $1`, tenant); total != esperado {
			t.Errorf("CASCADE: %s deveria ficar com %d linhas, ficou com %d", tabela, esperado, total)
		}
	}
	for _, tabela := range []string{"code_memory", "remote_action_attempts"} {
		if total := contar(t, `SELECT count(*) FROM `+tabela+` WHERE tenant_id = $1 AND task_run_id IS NULL`, tenant); total != 1 {
			t.Errorf("SET NULL: %s deveria manter a linha com task_run_id nulo, achei %d", tabela, total)
		}
	}
}

func TestGateEAprovacaoImpedemApagarOTaskRun(t *testing.T) {
	repo := exigirBanco(t)

	comGate := novoTaskRun(t, repo, novoTenant())
	if err := repo.GravarGateDeRisco(t.Context(), domain.GateDeRisco{
		TaskRunID: comGate.ID, NivelDeterministico: domain.RiscoAlto, Nivel: domain.RiscoAlto, Motivo: "dependencia_nova",
	}); err != nil {
		t.Fatalf("gravar gate: %v", err)
	}
	if err := repo.GravarGateDeRisco(t.Context(), domain.GateDeRisco{
		TaskRunID: comGate.ID, NivelDeterministico: domain.RiscoBaixo, Nivel: domain.RiscoBaixo, Motivo: "outro",
	}); !errors.Is(err, domain.ErrDuplicado) {
		t.Errorf("um segundo gate para o mesmo task_run deveria dar ErrDuplicado, obtive %v", err)
	}

	comAprovacao := novoTaskRun(t, repo, novoTenant())
	registrarRevisao(t, repo, comAprovacao, "aprovacao-"+comAprovacao.ID)

	for nome, taskRun := range map[string]string{"risk_gates": comGate.ID, "approvals": comAprovacao.ID} {
		_, err := pool.Exec(t.Context(), `DELETE FROM task_runs WHERE id = $1`, taskRun)
		if codigoPostgres(err) != "23503" {
			t.Errorf("RESTRICT em %s: esperava foreign_key_violation (23503), obtive %v", nome, err)
		}
	}
}

func registrarRevisao(t *testing.T, repo *repository.RepositorioPostgres, taskRun domain.TaskRun, deliveryID string) {
	t.Helper()

	err := repo.RegistrarRevisao(t.Context(), domain.DecisaoDeReview{
		InstallationID: taskRun.TenantID, DeliveryID: deliveryID, Repositorio: taskRun.Repositorio,
		NumeroPR: taskRun.NumeroPR, Estado: "approved", Revisor: "revisora", RecebidoEm: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("registrar revisão: %v", err)
	}
}

func TestRegistrarRevisaoVinculaAoTaskRunMaisRecenteENaoDuplica(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()
	antigo := novoTaskRun(t, repo, tenant)
	recente := novoTaskRun(t, repo, tenant)

	registrarRevisao(t, repo, recente, "revisao-"+recente.ID)
	registrarRevisao(t, repo, recente, "revisao-"+recente.ID)

	if total := contar(t, `SELECT count(*) FROM approvals WHERE task_run_id = $1 AND decision = 'aprovada'`, recente.ID); total != 1 {
		t.Errorf("esperava 1 aprovação no task_run mais recente, obtive %d", total)
	}
	if total := contar(t, `SELECT count(*) FROM approvals WHERE task_run_id = $1`, antigo.ID); total != 0 {
		t.Errorf("a aprovação não deveria ir para o task_run antigo, foram %d", total)
	}
}

func TestRegistrarAprovacaoDeTaskRunInexistente(t *testing.T) {
	repo := exigirBanco(t)

	err := repo.RegistrarAprovacao(t.Context(), domain.Aprovacao{
		TaskRunID: "00000000-0000-0000-0000-000000000000", DeliveryID: "aprovacao-orfa", Decisao: domain.DecisaoAprovada,
		Revisor: "revisora", DecididaEm: time.Now().UTC(),
	})
	if !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func TestRegistrarRevisaoSemTaskRunDevolveNaoEncontrado(t *testing.T) {
	repo := exigirBanco(t)

	err := repo.RegistrarRevisao(t.Context(), domain.DecisaoDeReview{
		InstallationID: novoTenant(), DeliveryID: "revisao-orfa", Repositorio: "Azulpasta/alvo",
		NumeroPR: 99, Estado: "changes_requested", Revisor: "revisora", RecebidoEm: time.Now().UTC(),
	})
	if !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func vetorUnitario(eixo int) []float32 {
	vetor := make([]float32, domain.DimensoesEmbedding)
	vetor[eixo] = 1
	return vetor
}

func memoria(tenant int64, taskRunID *string, embedding []float32) domain.MemoriaDeCodigo {
	return domain.MemoriaDeCodigo{
		TenantID: tenant, TaskRunID: taskRunID, Repositorio: "Azulpasta/alvo", Resumo: "mudança",
		Embedding: embedding, Nivel: domain.RiscoAlto, Desfecho: domain.DesfechoSemReversao,
	}
}

func TestBuscaVetorialDevolveOVizinhoMaisProximo(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()

	quaseX := vetorUnitario(0)
	quaseX[1] = 0.1
	diagonal := vetorUnitario(0)
	diagonal[1] = 1

	esperado, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(tenant, nil, quaseX))
	if err != nil {
		t.Fatalf("gravar: %v", err)
	}
	for _, outro := range [][]float32{vetorUnitario(1), diagonal, vetorUnitario(2)} {
		if _, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(tenant, nil, outro)); err != nil {
			t.Fatalf("gravar: %v", err)
		}
	}
	if _, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(novoTenant(), nil, vetorUnitario(0))); err != nil {
		t.Fatalf("gravar em outro tenant: %v", err)
	}

	vizinhos, err := repo.BuscarVizinhos(t.Context(), tenant, vetorUnitario(0), 2)
	if err != nil {
		t.Fatalf("buscar vizinhos: %v", err)
	}
	if len(vizinhos) != 2 {
		t.Fatalf("esperava 2 vizinhos, obtive %d", len(vizinhos))
	}
	if vizinhos[0].ID != esperado {
		t.Errorf("o mais próximo deveria ser %d, obtive %+v", esperado, vizinhos)
	}
	if vizinhos[0].Distancia > vizinhos[1].Distancia {
		t.Errorf("vizinhos fora de ordem: %+v", vizinhos)
	}
	if math.Abs(vizinhos[0].Distancia-(1-1/math.Sqrt(1.01))) > 1e-4 {
		t.Errorf("distância de cosseno inesperada: %v", vizinhos[0].Distancia)
	}
}

func TestMemoriaDeCodigoRecusaTaskRunDeOutroTenant(t *testing.T) {
	repo := exigirBanco(t)
	alheio := novoTaskRun(t, repo, novoTenant())

	if _, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(novoTenant(), &alheio.ID, vetorUnitario(0))); !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado ao misturar tenants, obtive %v", err)
	}
}

func TestBuscaVetorialAchaOsVizinhosDeOutroTenantPeloHNSW(t *testing.T) {
	repo := exigirBanco(t)
	vizinho, consultado := novoTenant(), novoTenant()
	aleatorio := rand.New(rand.NewPCG(1, 2))

	// O outro tenant ocupa toda a vizinhança da consulta. Sem varredura
	// iterativa, o HNSW entrega só os ef_search candidatos mais próximos, todos
	// do outro tenant, e o filtro por tenant esvazia o resultado.
	gravar := func(tenant int64, perto bool) {
		vetor := make([]float32, domain.DimensoesEmbedding)
		for i := range vetor {
			vetor[i] = aleatorio.Float32() - 0.5
		}
		if perto {
			vetor[0] = 30
		}
		if _, err := repo.GravarMemoriaDeCodigo(t.Context(), memoria(tenant, nil, vetor)); err != nil {
			t.Fatalf("gravar memória: %v", err)
		}
	}
	for range 200 {
		gravar(vizinho, true)
	}
	for range 20 {
		gravar(consultado, false)
	}
	if _, err := pool.Exec(t.Context(), `ANALYZE code_memory`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	vizinhos, err := repositorioPeloHNSW(t).BuscarVizinhos(t.Context(), consultado, vetorUnitario(0), 5)
	if err != nil {
		t.Fatalf("buscar vizinhos: %v", err)
	}
	if len(vizinhos) != 5 {
		t.Errorf("o tenant consultado tem 20 memórias e deveria receber 5, recebeu %d", len(vizinhos))
	}
}

func TestBuscaVetorialRecusaDimensaoErrada(t *testing.T) {
	repo := exigirBanco(t)

	if _, err := repo.BuscarVizinhos(t.Context(), novoTenant(), []float32{1, 2, 3}, 1); !errors.Is(err, domain.ErrDadosInvalidos) {
		t.Errorf("esperava ErrDadosInvalidos, obtive %v", err)
	}
}

func TestResolverVinculoDeIdentidade(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()

	if err := repo.VincularIdentidade(t.Context(), tenant, "U123", "revisora"); err != nil {
		t.Fatalf("vincular: %v", err)
	}

	login, err := repo.ResolverIdentidade(t.Context(), tenant, "U123")
	if err != nil || login != "revisora" {
		t.Errorf("esperava revisora, obtive %q %v", login, err)
	}
	if _, err := repo.ResolverIdentidade(t.Context(), novoTenant(), "U123"); !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("o vínculo não deveria vazar para outro tenant, obtive %v", err)
	}
}

func TestColetorPostgresEntregaOResultadoGravado(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())
	resultado := sandbox.ResultadoSandbox{
		Compilou: true, TestesPassaram: 12, TestesFalharam: 1, SaidaBruta: "--- FAIL", DuracaoMS: 4200, TipoFalha: sandbox.FalhaTeste,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = repo.Gravar(context.Background(), taskRun.ID, resultado)
	}()

	ctx, cancelar := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelar()
	obtido, err := repo.Aguardar(ctx, taskRun.ID)
	if err != nil {
		t.Fatalf("aguardar: %v", err)
	}
	if obtido != resultado {
		t.Errorf("esperava %+v, obtive %+v", resultado, obtido)
	}

	if tentativa := contar(t, `SELECT attempt FROM sandbox_results WHERE task_run_id = $1`, taskRun.ID); tentativa != 1 {
		t.Errorf("o coletor deveria gravar a tentativa 1, gravou %d", tentativa)
	}

	if err := repo.Gravar(t.Context(), taskRun.ID, resultado); !errors.Is(err, sandbox.ErrResultadoJaGravado) {
		t.Errorf("segunda gravação deveria dar ErrResultadoJaGravado, obtive %v", err)
	}
}

func TestColetorPostgresRespeitaOPrazo(t *testing.T) {
	repo := exigirBanco(t)
	taskRun := novoTaskRun(t, repo, novoTenant())

	ctx, cancelar := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancelar()
	if _, err := repo.Aguardar(ctx, taskRun.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("esperava o erro do contexto, obtive %v", err)
	}
}

func TestColetorPostgresRecusaTaskRunInexistente(t *testing.T) {
	repo := exigirBanco(t)

	err := repo.Gravar(t.Context(), "00000000-0000-0000-0000-000000000000", sandbox.ResultadoSandbox{TipoFalha: sandbox.FalhaNenhuma})
	if !errors.Is(err, domain.ErrNaoEncontrado) {
		t.Errorf("esperava ErrNaoEncontrado, obtive %v", err)
	}
}

func tentativaDeExemplo(tenant int64, idAcao string) acaoremota.Tentativa {
	return acaoremota.Tentativa{
		IDAcao: idAcao, Tipo: acaoremota.TipoAprovar, UsuarioSlack: "U123", LoginGitHub: "revisora",
		Repositorio: "Azulpasta/alvo", NumeroPR: 7, InstallationID: tenant,
		Decisao: acaoremota.DecisaoAceita, Motivo: acaoremota.MotivoAutorizada, Em: time.Now().UTC(),
	}
}

func TestRegistroDeTentativasMantemOPrimeiroCliqueEVinculaOTaskRun(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()
	taskRun := novoTaskRun(t, repo, tenant)
	idAcao := "clique-" + taskRun.ID

	if err := repo.Registrar(t.Context(), tentativaDeExemplo(tenant, idAcao)); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	repetida := tentativaDeExemplo(tenant, idAcao)
	repetida.Decisao, repetida.Motivo = acaoremota.DecisaoRepetida, acaoremota.MotivoAcaoRepetida
	if err := repo.Registrar(t.Context(), repetida); err != nil {
		t.Fatalf("registrar repetida: %v", err)
	}

	var decisao, taskRunID string
	err := pool.QueryRow(t.Context(), `SELECT decision, task_run_id FROM remote_action_attempts WHERE action_id = $1`, idAcao).Scan(&decisao, &taskRunID)
	if err != nil {
		t.Fatalf("ler tentativa: %v", err)
	}
	if decisao != string(acaoremota.DecisaoAceita) || taskRunID != taskRun.ID {
		t.Errorf("esperava a primeira decisão vinculada ao task_run %s, obtive %s %s", taskRun.ID, decisao, taskRunID)
	}
}

func TestRegistroDeTentativasIgnoraPedidoInvalido(t *testing.T) {
	repo := exigirBanco(t)

	invalida := acaoremota.Tentativa{IDAcao: "invalida-" + time.Now().Format(time.RFC3339Nano), Decisao: acaoremota.DecisaoInvalida, Motivo: acaoremota.MotivoCorpoInvalido}
	if err := repo.Registrar(t.Context(), invalida); err != nil {
		t.Fatalf("pedido inválido não deveria falhar o registro: %v", err)
	}
	if total := contar(t, `SELECT count(*) FROM remote_action_attempts WHERE action_id = $1`, invalida.IDAcao); total != 0 {
		t.Errorf("pedido inválido não deveria ir para o banco")
	}
}

// repositorioPeloHNSW usa conexões em que o planejador não pode ordenar por
// conta própria, o que obriga a busca a passar pelo índice HNSW em vez do
// índice de tenant_id, como aconteceria numa tabela grande.
func repositorioPeloHNSW(t *testing.T) *repository.RepositorioPostgres {
	t.Helper()

	configuracao, err := pgxpool.ParseConfig(urlBase)
	if err != nil {
		t.Fatalf("configurar pool: %v", err)
	}
	configuracao.ConnConfig.RuntimeParams["enable_sort"] = "off"

	poolComIndice, err := pgxpool.NewWithConfig(t.Context(), configuracao)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(poolComIndice.Close)

	return repository.NovoRepositorioPostgres(poolComIndice, repository.OpcoesPostgres{})
}
