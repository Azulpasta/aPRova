package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/sandbox/jobsazure"
)

const (
	intervaloDeConsulta  = 5 * time.Second
	tempoLimiteDoDisparo = 15 * time.Minute
)

func main() {
	if err := executar(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func executar() error {
	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}

	cliente, err := jobsazure.NovoCliente(jobsazure.Credenciais{
		SubscriptionID: configuracao.AzureSubscriptionID,
		TenantID:       configuracao.AzureTenantID,
		ClientID:       configuracao.AzureClientID,
		ClientSecret:   configuracao.AzureClientSecret,
		ResourceGroup:  configuracao.AzureResourceGroup,
		NomeDoJob:      configuracao.AzureNomeDoJobSandbox,
	})
	if err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteDoDisparo)
	defer cancelar()

	idExecucao, err := cliente.Iniciar(ctx, variaveisDeVerificacao())
	if err != nil {
		return err
	}
	fmt.Printf("execução disparada: %s\n", idExecucao)

	statusFinal, err := acompanhar(ctx, cliente, idExecucao)
	if err != nil {
		return err
	}

	fmt.Printf("status final: %s\n", statusFinal)
	if statusFinal != jobsazure.StatusConcluido {
		return fmt.Errorf("a execução terminou em %s", statusFinal)
	}
	return nil
}

func acompanhar(ctx context.Context, cliente *jobsazure.Cliente, idExecucao string) (string, error) {
	ultimo := ""

	for {
		status, err := cliente.Status(ctx, idExecucao)
		if err != nil {
			return "", err
		}
		if status != ultimo {
			fmt.Printf("  %s  %s\n", time.Now().Format(time.TimeOnly), status)
			ultimo = status
		}
		if jobsazure.StatusTerminal(status) {
			return status, nil
		}

		select {
		case <-ctx.Done():
			return ultimo, fmt.Errorf("prazo de %v esgotado com a execução em %s", tempoLimiteDoDisparo, ultimo)
		case <-time.After(intervaloDeConsulta):
		}
	}
}

func variaveisDeVerificacao() map[string]string {
	return map[string]string{
		"APROVA_REPOSITORIO": "https://github.com/Azulpasta/aPRova.git",
		"APROVA_SHA":         "0000000000000000000000000000000000000000",
		"APROVA_LINGUAGEM":   "go",
		"APROVA_TASK_RUN_ID": "verificacao-manual-" + time.Now().UTC().Format("20060102T150405Z"),
	}
}
