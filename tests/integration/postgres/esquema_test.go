package integracaopostgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

var tabelasDoNucleo = []string{
	"task_runs", "task_steps", "sandbox_results", "risk_gates", "receipts",
	"approvals", "code_memory", "identity_links", "remote_action_attempts",
}

func TestCadaMigrationAplicaEReverteLimpa(t *testing.T) {
	exigirBanco(t)

	url := criarBancoVazio(t, "migracoes_ida_e_volta")
	migracao, err := novaMigracao(url)
	if err != nil {
		t.Fatalf("abrir migrate: %v", err)
	}
	defer fecharMigracao(migracao)

	passos := 0
	for {
		esquemaAntes := retratoDoEsquema(t, url)

		if err := migracao.Steps(1); err != nil {
			if strings.Contains(err.Error(), "file does not exist") {
				break
			}
			t.Fatalf("subir migration %d: %v", passos+1, err)
		}
		if err := migracao.Steps(-1); err != nil {
			t.Fatalf("reverter migration %d: %v", passos+1, err)
		}
		if depois := retratoDoEsquema(t, url); depois != esquemaAntes {
			t.Fatalf("o down da migration %d não desfez o up:\nantes:  %s\ndepois: %s", passos+1, esquemaAntes, depois)
		}
		if err := migracao.Steps(1); err != nil {
			t.Fatalf("reaplicar migration %d: %v", passos+1, err)
		}
		passos++
	}

	if passos != 10 {
		t.Errorf("esperava 10 migrations, apliquei %d", passos)
	}

	if err := migracao.Down(); err != nil {
		t.Fatalf("down até zero: %v", err)
	}
	if sobrou := retratoDoEsquema(t, url); sobrou != "" {
		t.Errorf("down até zero deixou objetos para trás: %s", sobrou)
	}
}

func TestTodasAsTabelasTemTenantIDObrigatorioEIndexado(t *testing.T) {
	exigirBanco(t)

	for _, tabela := range tabelasDoNucleo {
		t.Run(tabela, func(t *testing.T) {
			obrigatorio := contar(t, `
				SELECT count(*) FROM information_schema.columns
				WHERE table_name = $1 AND column_name = 'tenant_id' AND is_nullable = 'NO'`, tabela)
			if obrigatorio != 1 {
				t.Errorf("%s.tenant_id deveria existir e ser NOT NULL", tabela)
			}

			indices := contar(t, `
				SELECT count(*) FROM pg_indexes
				WHERE tablename = $1 AND indexdef LIKE '%(tenant_id)%'`, tabela)
			if indices == 0 {
				t.Errorf("%s não tem índice em tenant_id", tabela)
			}
		})
	}
}

func TestEmbeddingTemIndiceHNSW(t *testing.T) {
	exigirBanco(t)

	if contar(t, `SELECT count(*) FROM pg_indexes WHERE tablename = 'code_memory' AND indexdef LIKE '%USING hnsw (embedding%'`) != 1 {
		t.Error("code_memory.embedding deveria ter índice HNSW")
	}
}

func TestChecksRecusamValoresForaDaLista(t *testing.T) {
	repo := exigirBanco(t)
	tenant := novoTenant()
	taskRun := novoTaskRun(t, repo, tenant)

	casos := map[string]string{
		"state":        `UPDATE task_runs SET state = 'inventado' WHERE id = $1`,
		"level":        `INSERT INTO risk_gates (tenant_id, task_run_id, deterministic_level, level, reason) VALUES (1, $1, 'alto', 'altissimo', 'x')`,
		"failure_type": `INSERT INTO sandbox_results (tenant_id, task_run_id, attempt, compiled, tests_passed, tests_failed, raw_output, duration_ms, failure_type) VALUES (1, $1, 1, true, 0, 0, '', 0, 'culpa_do_estagiario')`,
		"decision":     `INSERT INTO approvals (tenant_id, task_run_id, delivery_id, decision, reviewer, decided_at) VALUES (1, $1, 'x', 'talvez', 'r', now())`,
	}

	for coluna, comando := range casos {
		t.Run(coluna, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), comando, taskRun.ID)
			if codigoPostgres(err) != "23514" {
				t.Errorf("esperava check_violation (23514) em %s, obtive %v", coluna, err)
			}
		})
	}
}

func criarBancoVazio(t *testing.T, nome string) string {
	t.Helper()

	if _, err := pool.Exec(t.Context(), "CREATE DATABASE "+pgx.Identifier{nome}.Sanitize()); err != nil {
		t.Fatalf("criar banco %s: %v", nome, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{nome}.Sanitize()+" WITH (FORCE)")
	})

	inicio, resto, _ := strings.Cut(urlBase, "?")
	return inicio[:strings.LastIndex(inicio, "/")+1] + nome + "?" + resto
}

// retratoDoEsquema descreve tabelas, colunas, índices, funções, triggers e
// extensões do banco, ignorando o controle de versão do próprio migrate.
func retratoDoEsquema(t *testing.T, url string) string {
	t.Helper()

	conexao, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatalf("conectar em %s: %v", url, err)
	}
	defer func() { _ = conexao.Close(context.Background()) }()

	var retrato string
	err = conexao.QueryRow(t.Context(), `
		SELECT coalesce(string_agg(item, ' | ' ORDER BY item), '') FROM (
			SELECT 'coluna ' || table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable AS item
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
			UNION ALL
			SELECT 'indice ' || indexname FROM pg_indexes
			WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
			UNION ALL
			SELECT 'funcao ' || p.proname FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public' AND NOT EXISTS (
				SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
			UNION ALL
			SELECT 'trigger ' || tgname FROM pg_trigger WHERE NOT tgisinternal
			UNION ALL
			SELECT 'extensao ' || extname FROM pg_extension WHERE extname <> 'plpgsql'
		) itens`).Scan(&retrato)
	if err != nil {
		t.Fatalf("retratar esquema: %v", err)
	}
	return retrato
}

func TestRetratoEnxergaOEsquemaCompleto(t *testing.T) {
	exigirBanco(t)

	retrato := retratoDoEsquema(t, urlBase)
	for _, esperado := range append(tabelasDoNucleo, "receipts_append_only", "extensao vector") {
		if !strings.Contains(retrato, esperado) {
			t.Errorf("o retrato do esquema deveria mencionar %q", esperado)
		}
	}
	if t.Failed() {
		t.Logf("retrato: %s", retrato)
	}
}
