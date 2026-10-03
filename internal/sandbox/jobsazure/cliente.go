package jobsazure

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v4"
)

const frequenciaDaOperacao = 2 * time.Second

// Credenciais identifica o service principal e o job do sandbox na Azure.
type Credenciais struct {
	SubscriptionID string
	TenantID       string
	ClientID       string
	ClientSecret   string
	ResourceGroup  string
	NomeDoJob      string
}

// Cliente dispara e acompanha execuções de um Container Apps Job. Sua
// superfície usa apenas tipos da biblioteca padrão.
type Cliente struct {
	jobs          *armappcontainers.JobsClient
	api           *armappcontainers.ContainerAppsAPIClient
	resourceGroup string
	nomeDoJob     string
}

// NovoCliente autentica por service principal e constrói o cliente. Nenhuma
// chamada de rede acontece aqui; o token é obtido na primeira operação.
func NovoCliente(credenciais Credenciais) (*Cliente, error) {
	credencial, err := azidentity.NewClientSecretCredential(
		credenciais.TenantID, credenciais.ClientID, credenciais.ClientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("criar credencial do service principal: %w", err)
	}

	fabrica, err := armappcontainers.NewClientFactory(credenciais.SubscriptionID, credencial, nil)
	if err != nil {
		return nil, fmt.Errorf("criar clientes do container apps: %w", err)
	}

	return &Cliente{
		jobs:          fabrica.NewJobsClient(),
		api:           fabrica.NewContainerAppsAPIClient(),
		resourceGroup: credenciais.ResourceGroup,
		nomeDoJob:     credenciais.NomeDoJob,
	}, nil
}

// Iniciar dispara uma execução do job sobrescrevendo as variáveis informadas
// e preservando o restante da definição dos containers. Devolve o nome da
// execução.
func (c *Cliente) Iniciar(ctx context.Context, variaveis map[string]string) (string, error) {
	modelo, err := c.containersDoJob(ctx)
	if err != nil {
		return "", err
	}

	poller, err := c.jobs.BeginStart(ctx, c.resourceGroup, c.nomeDoJob, &armappcontainers.JobsClientBeginStartOptions{
		Template: &armappcontainers.JobExecutionTemplate{Containers: containersDaExecucao(modelo, variaveis)},
	})
	if err != nil {
		return "", fmt.Errorf("disparar execução do job: %w", err)
	}

	resposta, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: frequenciaDaOperacao})
	if err != nil {
		return "", fmt.Errorf("aguardar criação da execução: %w", err)
	}
	if resposta.Name == nil {
		return "", errors.New("a azure não devolveu o nome da execução")
	}

	return *resposta.Name, nil
}

// Status devolve o estado atual da execução como a Azure o reporta, por
// exemplo Running, Succeeded ou Failed.
func (c *Cliente) Status(ctx context.Context, idExecucao string) (string, error) {
	resposta, err := c.api.JobExecution(ctx, c.resourceGroup, c.nomeDoJob, idExecucao, nil)
	if err != nil {
		return "", fmt.Errorf("consultar execução %s: %w", idExecucao, err)
	}
	if resposta.Properties == nil || resposta.Properties.Status == nil {
		return StatusDesconhecido, nil
	}

	return string(*resposta.Properties.Status), nil
}

// Parar interrompe a execução e aguarda a confirmação da Azure.
func (c *Cliente) Parar(ctx context.Context, idExecucao string) error {
	poller, err := c.jobs.BeginStopExecution(ctx, c.resourceGroup, c.nomeDoJob, idExecucao, nil)
	if err != nil {
		return fmt.Errorf("parar execução %s: %w", idExecucao, err)
	}

	if _, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: frequenciaDaOperacao}); err != nil {
		return fmt.Errorf("aguardar parada da execução %s: %w", idExecucao, err)
	}
	return nil
}

func (c *Cliente) containersDoJob(ctx context.Context) ([]*armappcontainers.Container, error) {
	resposta, err := c.jobs.Get(ctx, c.resourceGroup, c.nomeDoJob, nil)
	if err != nil {
		return nil, fmt.Errorf("ler definição do job %s: %w", c.nomeDoJob, err)
	}
	if resposta.Properties == nil || resposta.Properties.Template == nil || len(resposta.Properties.Template.Containers) == 0 {
		return nil, fmt.Errorf("o job %s não tem containers definidos", c.nomeDoJob)
	}

	return resposta.Properties.Template.Containers, nil
}

func containersDaExecucao(modelo []*armappcontainers.Container, variaveis map[string]string) []*armappcontainers.JobExecutionContainer {
	execucao := make([]*armappcontainers.JobExecutionContainer, 0, len(modelo))
	for _, container := range modelo {
		execucao = append(execucao, &armappcontainers.JobExecutionContainer{
			Name:      container.Name,
			Image:     container.Image,
			Command:   container.Command,
			Args:      container.Args,
			Resources: container.Resources,
			Env:       mesclarVariaveis(container.Env, variaveis),
		})
	}
	return execucao
}

func mesclarVariaveis(doJob []*armappcontainers.EnvironmentVar, doPedido map[string]string) []*armappcontainers.EnvironmentVar {
	mescladas := make([]*armappcontainers.EnvironmentVar, 0, len(doJob)+len(doPedido))

	for _, variavel := range doJob {
		if variavel.Name == nil {
			continue
		}
		if _, sobrescrita := doPedido[*variavel.Name]; !sobrescrita {
			mescladas = append(mescladas, variavel)
		}
	}

	nomes := make([]string, 0, len(doPedido))
	for nome := range doPedido {
		nomes = append(nomes, nome)
	}
	slices.Sort(nomes)

	for _, nome := range nomes {
		valor := doPedido[nome]
		mescladas = append(mescladas, &armappcontainers.EnvironmentVar{Name: &nome, Value: &valor})
	}

	return mescladas
}
