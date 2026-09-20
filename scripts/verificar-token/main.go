package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/platform"
)

const tempoLimiteVerificacao = 30 * time.Second

func main() {
	if err := executar(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func executar() error {
	instalacaoID, err := lerInstalacaoID(os.Args)
	if err != nil {
		return err
	}

	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}

	autenticador, err := platform.NovoAutenticador(platform.OpcoesAutenticador{
		AppID:           configuracao.GitHubAppID,
		ChavePrivadaPEM: configuracao.GitHubChavePrivada,
	})
	if err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteVerificacao)
	defer cancelar()

	token, err := autenticador.TokenParaInstalacao(ctx, instalacaoID)
	if err != nil {
		return explicarFalha(err)
	}

	fmt.Printf("token de instalação obtido: %s… (%d caracteres)\n", token[:min(8, len(token))], len(token))

	restantes, limite, err := consultarSaldo(ctx, token)
	if err != nil {
		return err
	}

	fmt.Printf("saldo de requisições: %d de %d restantes nesta hora\n", restantes, limite)

	return nil
}

func lerInstalacaoID(argumentos []string) (int64, error) {
	if len(argumentos) < 2 {
		return 0, fmt.Errorf("uso: %s <installation_id>", argumentos[0])
	}

	instalacaoID, err := strconv.ParseInt(argumentos[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("installation_id precisa ser numérico, recebi %q", argumentos[1])
	}

	return instalacaoID, nil
}

func consultarSaldo(ctx context.Context, token string) (restantes, limite int, err error) {
	requisicao, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/rate_limit", nil)
	if err != nil {
		return 0, 0, fmt.Errorf("montar requisição de saldo: %w", err)
	}
	requisicao.Header.Set("Authorization", "Bearer "+token)
	requisicao.Header.Set("Accept", "application/vnd.github+json")

	resposta, err := http.DefaultClient.Do(requisicao)
	if err != nil {
		return 0, 0, fmt.Errorf("consultar saldo: %w", err)
	}
	defer func() { _ = resposta.Body.Close() }()

	if resposta.StatusCode >= http.StatusMultipleChoices {
		return 0, 0, fmt.Errorf("github respondeu %d ao consultar o saldo", resposta.StatusCode)
	}

	var corpo struct {
		Recursos struct {
			Central struct {
				Limite    int `json:"limit"`
				Restantes int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resposta.Body).Decode(&corpo); err != nil {
		return 0, 0, fmt.Errorf("interpretar saldo: %w", err)
	}

	return corpo.Recursos.Central.Restantes, corpo.Recursos.Central.Limite, nil
}

func explicarFalha(err error) error {
	switch {
	case errors.Is(err, platform.ErrConfiguracaoInvalida):
		return fmt.Errorf("%w\nconfira GITHUB_APP_ID e GITHUB_PRIVATE_KEY_BASE64 no .env", err)
	case errors.Is(err, platform.ErrAppNaoInstalado):
		return fmt.Errorf("%w\ninstale o app no repositório e use o installation_id do evento", err)
	case errors.Is(err, platform.ErrCredencialRejeitada):
		return fmt.Errorf("%w\nverifique o relógio da máquina e se a chave pertence a este app", err)
	default:
		return err
	}
}
