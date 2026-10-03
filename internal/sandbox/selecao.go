package sandbox

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azulpasta/aPRova/internal/sandbox/jobsazure"
)

const (
	tipoExecutorLocal = "local"
	tipoExecutorAzure = "azure"
)

// ErrConfiguracaoSandbox indica configuração de executor que impede o boot.
var ErrConfiguracaoSandbox = errors.New("sandbox: configuração do executor inválida")

// ConfiguracaoExecutor reúne o necessário para escolher e construir o
// executor do sandbox na inicialização.
type ConfiguracaoExecutor struct {
	Tipo    string
	Local   OpcoesExecutorLocal
	Azure   OpcoesExecutorAzure
	Coletor ColetorResultado
}

// NovoExecutor escolhe o executor pelo tipo configurado em SANDBOX_EXECUTOR.
// Tipo ausente, desconhecido ou com configuração incompleta devolve
// ErrConfiguracaoSandbox.
func NovoExecutor(configuracao ConfiguracaoExecutor) (Executor, error) {
	switch configuracao.Tipo {
	case tipoExecutorLocal:
		return NovoExecutorLocal(configuracao.Local), nil
	case tipoExecutorAzure:
		return NovoExecutorAzure(configuracao.Azure, configuracao.Coletor)
	default:
		return nil, fmt.Errorf("%w: SANDBOX_EXECUTOR aceita %s ou %s, obtive %q",
			ErrConfiguracaoSandbox, tipoExecutorLocal, tipoExecutorAzure, configuracao.Tipo)
	}
}

// NovoExecutorAzure constrói o executor que dispara o job do sandbox no Azure
// Container Apps. Exige todas as credenciais e um coletor de resultados.
func NovoExecutorAzure(opcoes OpcoesExecutorAzure, coletor ColetorResultado) (Executor, error) {
	if ausentes := variaveisAzureAusentes(opcoes); len(ausentes) > 0 {
		return nil, fmt.Errorf("%w: o executor azure exige %s", ErrConfiguracaoSandbox, strings.Join(ausentes, ", "))
	}
	if coletor == nil {
		return nil, fmt.Errorf("%w: o executor azure exige um coletor de resultados", ErrConfiguracaoSandbox)
	}

	cliente, err := jobsazure.NovoCliente(jobsazure.Credenciais{
		SubscriptionID: opcoes.SubscriptionID,
		TenantID:       opcoes.TenantID,
		ClientID:       opcoes.ClientID,
		ClientSecret:   opcoes.ClientSecret,
		ResourceGroup:  opcoes.ResourceGroup,
		NomeDoJob:      opcoes.NomeDoJob,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfiguracaoSandbox, err)
	}

	return novoExecutorAzure(opcoes, cliente, coletor), nil
}

func variaveisAzureAusentes(opcoes OpcoesExecutorAzure) []string {
	exigidas := []struct {
		nome  string
		valor string
	}{
		{"AZURE_SUBSCRIPTION_ID", opcoes.SubscriptionID},
		{"AZURE_TENANT_ID", opcoes.TenantID},
		{"AZURE_CLIENT_ID", opcoes.ClientID},
		{"AZURE_CLIENT_SECRET", opcoes.ClientSecret},
		{"AZURE_RESOURCE_GROUP", opcoes.ResourceGroup},
		{"AZURE_SANDBOX_JOB_NAME", opcoes.NomeDoJob},
	}

	var ausentes []string
	for _, exigida := range exigidas {
		if strings.TrimSpace(exigida.valor) == "" {
			ausentes = append(ausentes, exigida.nome)
		}
	}
	return ausentes
}
