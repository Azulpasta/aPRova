package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const tempoLimiteConexao = 5 * time.Second

// ConectarPostgres abre um pool de conexões com o PostgreSQL na URL informada e
// confirma o acesso antes de retornar. Em caso de falha o pool é fechado.
func ConectarPostgres(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("abrir pool do postgres: %w", err)
	}

	contextoVerificacao, cancelar := context.WithTimeout(ctx, tempoLimiteConexao)
	defer cancelar()

	if err := pool.Ping(contextoVerificacao); err != nil {
		pool.Close()
		return nil, fmt.Errorf("verificar conexão com o postgres: %w", err)
	}

	return pool, nil
}

// ConectarRedis cria um cliente Redis a partir da URL informada e confirma o
// acesso antes de retornar. Em caso de falha o cliente é fechado.
func ConectarRedis(ctx context.Context, url string) (*redis.Client, error) {
	opcoes, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("interpretar url do redis: %w", err)
	}

	cliente := redis.NewClient(opcoes)

	contextoVerificacao, cancelar := context.WithTimeout(ctx, tempoLimiteConexao)
	defer cancelar()

	if err := cliente.Ping(contextoVerificacao).Err(); err != nil {
		_ = cliente.Close()
		return nil, fmt.Errorf("verificar conexão com o redis: %w", err)
	}

	return cliente, nil
}
