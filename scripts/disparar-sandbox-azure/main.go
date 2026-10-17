package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/Azulpasta/aPRova/internal/sandbox/jobsazure"
)

const (
	intervaloDeConsulta  = 5 * time.Second
	tempoLimiteDoDisparo = 15 * time.Minute
	prazoDaExecucao      = "600"
)

var formatoDoSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

type disparo struct {
	repositorio string
	sha         string
	diretorio   string
}

func main() {
	if err := executar(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func executar() error {
	pedido, err := lerArgumentos(os.Args)
	if err != nil {
		return err
	}

	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}
	if err := exigirImagem(configuracao.SandboxImagemGo); err != nil {
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

	variaveis := variaveisDoDisparo(pedido, time.Now())
	idExecucao, err := cliente.Iniciar(ctx, configuracao.SandboxImagemGo, variaveis)
	if err != nil {
		return err
	}
	fmt.Printf("execução disparada: %s (task_run_id %s, imagem %s, diretório %q)\n",
		idExecucao, variaveis["APROVA_TASK_RUN_ID"], configuracao.SandboxImagemGo, pedido.diretorio)

	statusFinal, err := acompanhar(ctx, cliente, idExecucao)
	if err != nil {
		return err
	}

	fmt.Printf("status final: %s\n", statusFinal)
	if statusFinal != jobsazure.StatusConcluido {
		return fmt.Errorf("a execução terminou em %s, sem resultado entregue", statusFinal)
	}
	fmt.Println("o ResultadoSandbox é a linha JSON no stdout do container, visível nos logs do job")
	return nil
}

func lerArgumentos(argumentos []string) (disparo, error) {
	if len(argumentos) != 3 && len(argumentos) != 4 {
		return disparo{}, fmt.Errorf("uso: %s <url_https_do_repositorio> <sha_completo> [diretorio_do_projeto]", argumentos[0])
	}

	endereco, err := url.Parse(argumentos[1])
	if err != nil || endereco.Scheme != "https" || endereco.Host == "" {
		return disparo{}, fmt.Errorf("o repositório precisa ser uma URL https, recebi %q", argumentos[1])
	}

	sha := strings.ToLower(argumentos[2])
	if !formatoDoSHA.MatchString(sha) {
		return disparo{}, fmt.Errorf("o SHA precisa ter os 40 caracteres hexadecimais, recebi %q", argumentos[2])
	}

	pedido := disparo{repositorio: argumentos[1], sha: sha}
	if len(argumentos) == 4 {
		pedido.diretorio = argumentos[3]
	}
	if err := sandbox.ValidarDiretorioDoProjeto(pedido.diretorio); err != nil {
		return disparo{}, err
	}

	return pedido, nil
}

func variaveisDoDisparo(pedido disparo, momento time.Time) map[string]string {
	return map[string]string{
		"APROVA_REPOSITORIO":    pedido.repositorio,
		"APROVA_SHA":            pedido.sha,
		"APROVA_LINGUAGEM":      "go",
		"APROVA_TASK_RUN_ID":    "verificacao-manual-" + momento.UTC().Format("20060102T150405Z"),
		"APROVA_PRAZO_SEGUNDOS": prazoDaExecucao,
		"APROVA_SAIDA":          "stdout",
		"APROVA_DIRETORIO":      pedido.diretorio,
	}
}

func exigirImagem(imagem string) error {
	if strings.Contains(imagem, "@sha256:") {
		return nil
	}

	_, tag, temTag := strings.Cut(path.Base(imagem), ":")
	if imagem == "" || !temTag || tag == "" || tag == "latest" {
		return fmt.Errorf("SANDBOX_IMAGEM_GO precisa apontar uma imagem com a tag do SHA do commit, recebi %q", imagem)
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
