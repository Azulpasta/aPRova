package repository

import (
	"context"
	"strings"
	"testing"
)

func TestConectarRedisRejeitaURLMalformada(t *testing.T) {
	cliente, err := ConectarRedis(context.Background(), "isto-nao-e-uma-url")

	if err == nil {
		t.Fatal("esperava erro para URL malformada, obtive nil")
	}
	if cliente != nil {
		t.Error("nenhum cliente deve ser devolvido junto com erro")
	}
	if !strings.Contains(err.Error(), "interpretar url do redis") {
		t.Errorf("o erro deveria dizer em que etapa falhou, obtive: %v", err)
	}
}

func TestConectarPostgresRejeitaURLMalformada(t *testing.T) {
	pool, err := ConectarPostgres(context.Background(), "isto-nao-e-uma-url")

	if err == nil {
		t.Fatal("esperava erro para URL malformada, obtive nil")
	}
	if pool != nil {
		t.Error("nenhum pool deve ser devolvido junto com erro")
	}
}

func TestConectarPostgresFalhaQuandoNaoHaServidorEscutando(t *testing.T) {
	pool, err := ConectarPostgres(context.Background(), "postgres://u:s@127.0.0.1:1/d?sslmode=disable")

	if err == nil {
		t.Fatal("esperava erro de conexão recusada, obtive nil")
	}
	if pool != nil {
		t.Error("o pool deve ser fechado e não devolvido quando o ping falha")
	}
	if !strings.Contains(err.Error(), "verificar conexão com o postgres") {
		t.Errorf("o erro deveria indicar que a falha foi na verificação, obtive: %v", err)
	}
}
