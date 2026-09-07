package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

const portaPadrao = "8080"

// Configuracao reúne os valores de ambiente exigidos pelos processos do aPRova.
type Configuracao struct {
	URLBancoDados        string `validate:"required"`
	URLRedis             string `validate:"required"`
	Porta                string `validate:"required,number"`
	GitHubAppID          string `validate:"required"`
	GitHubChavePrivada   string `validate:"required"`
	GitHubSegredoWebhook string `validate:"required"`
	ReciboChavePrivada   string `validate:"required"`
}

var variavelDeAmbientePorCampo = map[string]string{
	"URLBancoDados":        "DATABASE_URL",
	"URLRedis":             "REDIS_URL",
	"Porta":                "PORT",
	"GitHubAppID":          "GITHUB_APP_ID",
	"GitHubChavePrivada":   "GITHUB_PRIVATE_KEY",
	"GitHubSegredoWebhook": "GITHUB_WEBHOOK_SECRET",
	"ReciboChavePrivada":   "RECEIPT_PRIVATE_KEY",
}

// Carregar lê as variáveis de ambiente, aplica os valores padrão e valida o
// resultado. O erro retornado nomeia cada variável ausente ou inválida.
func Carregar() (Configuracao, error) {
	if err := carregarArquivoLocal(); err != nil {
		return Configuracao{}, err
	}

	configuracao := Configuracao{
		URLBancoDados:        lerVariavel("DATABASE_URL"),
		URLRedis:             lerVariavel("REDIS_URL"),
		Porta:                valorOuPadrao(lerVariavel("PORT"), portaPadrao),
		GitHubAppID:          lerVariavel("GITHUB_APP_ID"),
		GitHubChavePrivada:   lerVariavel("GITHUB_PRIVATE_KEY"),
		GitHubSegredoWebhook: lerVariavel("GITHUB_WEBHOOK_SECRET"),
		ReciboChavePrivada:   lerVariavel("RECEIPT_PRIVATE_KEY"),
	}

	if err := validar(configuracao); err != nil {
		return Configuracao{}, err
	}

	return configuracao, nil
}

// EnderecoHTTP devolve o endereço de escuta do servidor no formato aceito por
// net/http, derivado da porta configurada.
func (c Configuracao) EnderecoHTTP() string {
	return net.JoinHostPort("", c.Porta)
}

func carregarArquivoLocal() error {
	err := godotenv.Load()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("ler arquivo .env: %w", err)
}

func lerVariavel(nome string) string {
	return strings.TrimSpace(os.Getenv(nome))
}

func valorOuPadrao(valor, padrao string) string {
	if valor == "" {
		return padrao
	}
	return valor
}

func validar(configuracao Configuracao) error {
	err := validator.New().Struct(configuracao)
	if err == nil {
		return nil
	}

	var errosDeCampo validator.ValidationErrors
	if !errors.As(err, &errosDeCampo) {
		return fmt.Errorf("validar configuração: %w", err)
	}

	pendencias := make([]string, 0, len(errosDeCampo))
	for _, erroDeCampo := range errosDeCampo {
		pendencias = append(pendencias, descreverPendencia(erroDeCampo))
	}

	return fmt.Errorf("configuração inválida: %s", strings.Join(pendencias, "; "))
}

func nomeDaVariavel(campo string) string {
	if variavel, mapeado := variavelDeAmbientePorCampo[campo]; mapeado {
		return variavel
	}
	return campo
}

func descreverPendencia(erroDeCampo validator.FieldError) string {
	variavel := nomeDaVariavel(erroDeCampo.Field())
	if erroDeCampo.Tag() == "required" {
		return fmt.Sprintf("%s não definida", variavel)
	}
	return fmt.Sprintf("%s com valor inválido: esperado %s", variavel, erroDeCampo.Tag())
}
