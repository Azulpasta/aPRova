package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/repository"
	"github.com/Azulpasta/aPRova/internal/sandbox"
)

func main() {
	if err := executar(); err != nil {
		slog.Error("worker encerrado com erro", "erro", err)
		os.Exit(1)
	}
}

func executar() error {
	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}

	config.ConfigurarLogPadrao(configuracao)

	if _, err := sandbox.NovoExecutor(configuracaoDoSandbox(configuracao)); err != nil {
		return err
	}

	ctx, encerrarEscutaDeSinais := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer encerrarEscutaDeSinais()

	bancoDados, err := repository.ConectarPostgres(ctx, configuracao.URLBancoDados)
	if err != nil {
		return err
	}
	defer bancoDados.Close()

	clienteRedis, err := repository.ConectarRedis(ctx, configuracao.URLRedis)
	if err != nil {
		return err
	}
	defer func() {
		if err := clienteRedis.Close(); err != nil {
			slog.Error("falha ao fechar o cliente redis", "erro", err)
		}
	}()

	slog.Info("worker iniciado, aguardando execuções")
	<-ctx.Done()
	slog.Info("sinal de encerramento recebido, desligando o worker")

	return nil
}

func configuracaoDoSandbox(configuracao config.Configuracao) sandbox.ConfiguracaoExecutor {
	return sandbox.ConfiguracaoExecutor{
		Tipo: configuracao.SandboxExecutor,
		Azure: sandbox.OpcoesExecutorAzure{
			SubscriptionID: configuracao.AzureSubscriptionID,
			TenantID:       configuracao.AzureTenantID,
			ClientID:       configuracao.AzureClientID,
			ClientSecret:   configuracao.AzureClientSecret,
			ResourceGroup:  configuracao.AzureResourceGroup,
			NomeDoJob:      configuracao.AzureNomeDoJobSandbox,
		},
		Coletor: sandbox.NovoColetorEmMemoria(),
	}
}
