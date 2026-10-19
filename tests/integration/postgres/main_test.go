package integracaopostgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/domain"
	"github.com/Azulpasta/aPRova/internal/repository"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const imagemPostgres = "pgvector/pgvector:pg16"

var (
	urlBase       string
	pool          *pgxpool.Pool
	falhaAoSubir  error
	proximoTenant atomic.Int64
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	conteiner, err := subirPostgres(ctx)
	if err != nil {
		falhaAoSubir = err
		os.Exit(m.Run())
	}

	codigo := m.Run()

	pool.Close()
	if err := testcontainers.TerminateContainer(conteiner); err != nil {
		log.Printf("falha ao encerrar o container do postgres: %v", err)
	}
	os.Exit(codigo)
}

func subirPostgres(ctx context.Context) (*tcpostgres.PostgresContainer, error) {
	conteiner, err := tcpostgres.Run(ctx, imagemPostgres,
		tcpostgres.WithDatabase("aprova"),
		tcpostgres.WithUsername("aprova"),
		tcpostgres.WithPassword("aprova"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("subir postgres: %w", err)
	}

	urlBase, err = conteiner.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return conteiner, err
	}

	if err := aplicarMigrations(urlBase); err != nil {
		return conteiner, err
	}

	pool, err = repository.ConectarPostgres(ctx, urlBase)
	return conteiner, err
}

func aplicarMigrations(url string) error {
	migracao, err := novaMigracao(url)
	if err != nil {
		return err
	}
	defer fecharMigracao(migracao)

	if err := migracao.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}

func novaMigracao(url string) (*migrate.Migrate, error) {
	_, arquivo, _, _ := runtime.Caller(0)
	diretorio := filepath.Join(filepath.Dir(arquivo), "..", "..", "..", "migrations")

	urlPgx := "pgx5" + url[len("postgres"):]
	return migrate.New("file://"+filepath.ToSlash(diretorio), urlPgx)
}

func fecharMigracao(migracao *migrate.Migrate) {
	if errFonte, errBanco := migracao.Close(); errFonte != nil || errBanco != nil {
		log.Printf("fechar migrate: fonte=%v banco=%v", errFonte, errBanco)
	}
}

func exigirBanco(t *testing.T) *repository.RepositorioPostgres {
	t.Helper()

	if falhaAoSubir != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("postgres indisponível no CI: %v", falhaAoSubir)
		}
		t.Skipf("postgres indisponível (docker ausente?): %v", falhaAoSubir)
	}
	return repository.NovoRepositorioPostgres(pool, repository.OpcoesPostgres{IntervaloConsulta: 20 * time.Millisecond})
}

// novoTenant isola os dados de cada teste, que compartilham o mesmo banco.
func novoTenant() int64 {
	return 1_000_000 + proximoTenant.Add(1)
}

func novoTaskRun(t *testing.T, repo *repository.RepositorioPostgres, tenant int64) domain.TaskRun {
	t.Helper()

	criado, _, err := repo.CriarTaskRun(t.Context(), domain.NovoTaskRun{
		TenantID:    tenant,
		DeliveryID:  fmt.Sprintf("entrega-%d-%d", tenant, time.Now().UnixNano()),
		Repositorio: "Azulpasta/alvo",
		NumeroPR:    7,
		SHAHead:     "0123456789abcdef0123456789abcdef01234567",
		Autor:       "autora",
		Acao:        "opened",
		RecebidoEm:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("criar task_run: %v", err)
	}
	return criado
}

func codigoPostgres(err error) string {
	var erroPg *pgconn.PgError
	if errors.As(err, &erroPg) {
		return erroPg.Code
	}
	return ""
}

func contar(t *testing.T, consulta string, argumentos ...any) int {
	t.Helper()

	var total int
	if err := pool.QueryRow(t.Context(), consulta, argumentos...).Scan(&total); err != nil {
		t.Fatalf("contar (%s): %v", consulta, err)
	}
	return total
}
