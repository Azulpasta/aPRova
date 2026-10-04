package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const (
	portaPadrao    = "8080"
	nivelLogPadrao = "info"
	regiaoPadrao   = "us-central1"
)

const origemDocker = "padrão do docker-compose local, ver .env.example"

var nomesDosNiveisDeLog = []string{"debug", "info", "warn", "error"}

var niveisDeLogPorNome = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var variaveisQueSeraoObrigatorias = []string{
	"GITHUB_APP_ID",
	"GITHUB_PRIVATE_KEY_BASE64",
	"GITHUB_WEBHOOK_SECRET",
	"RECEIPT_PRIVATE_KEY_BASE64",
	"RECEIPT_PUBLIC_KEY_BASE64",
	"ANTHROPIC_API_KEY",
}

// Configuracao reúne os valores de ambiente exigidos pelos processos do aPRova.
type Configuracao struct {
	Porta    string
	NivelLog string

	URLBancoDados string
	URLRedis      string

	GitHubAppID          string
	GitHubChavePrivada   []byte
	GitHubSegredoWebhook string

	ReciboChavePrivada ed25519.PrivateKey
	ReciboChavePublica ed25519.PublicKey

	ChaveAPIAnthropic string

	GCPProjectID    string
	GCPRegiao       string
	SandboxExecutor string

	AzureSubscriptionID   string
	AzureTenantID         string
	AzureClientID         string
	AzureClientSecret     string
	AzureResourceGroup    string
	AzureNomeDoJobSandbox string

	URLWebhookN8N   string
	N8NSegredoAcoes string

	VinculosSlackGitHub map[string]string
}

// Carregar lê as variáveis de ambiente, aplica os valores padrão e valida o
// resultado. O erro retornado nomeia cada variável ausente ou inválida e onde
// obter o valor correto.
func Carregar() (Configuracao, error) {
	if err := carregarArquivoLocal(); err != nil {
		return Configuracao{}, err
	}

	var leitura leitor

	configuracao := Configuracao{
		Porta:    leitura.opcional("PORT", portaPadrao),
		NivelLog: leitura.nivelDeLog(),

		URLBancoDados: leitura.obrigatoria("DATABASE_URL", origemDocker),
		URLRedis:      leitura.obrigatoria("REDIS_URL", origemDocker),

		GitHubAppID:          leitura.opcional("GITHUB_APP_ID", ""),
		GitHubChavePrivada:   leitura.base64Opcional("GITHUB_PRIVATE_KEY_BASE64"),
		GitHubSegredoWebhook: leitura.opcional("GITHUB_WEBHOOK_SECRET", ""),

		ReciboChavePrivada: leitura.chavePrivadaEd25519("RECEIPT_PRIVATE_KEY_BASE64"),
		ReciboChavePublica: leitura.chavePublicaEd25519("RECEIPT_PUBLIC_KEY_BASE64"),

		ChaveAPIAnthropic: leitura.opcional("ANTHROPIC_API_KEY", ""),

		GCPProjectID:    leitura.opcional("GCP_PROJECT_ID", ""),
		GCPRegiao:       leitura.opcional("GCP_REGION", regiaoPadrao),
		SandboxExecutor: lerVariavel("SANDBOX_EXECUTOR"),

		AzureSubscriptionID:   lerVariavel("AZURE_SUBSCRIPTION_ID"),
		AzureTenantID:         lerVariavel("AZURE_TENANT_ID"),
		AzureClientID:         lerVariavel("AZURE_CLIENT_ID"),
		AzureClientSecret:     lerVariavel("AZURE_CLIENT_SECRET"),
		AzureResourceGroup:    lerVariavel("AZURE_RESOURCE_GROUP"),
		AzureNomeDoJobSandbox: lerVariavel("AZURE_SANDBOX_JOB_NAME"),

		URLWebhookN8N:   leitura.opcional("N8N_WEBHOOK_URL", ""),
		N8NSegredoAcoes: leitura.opcional("N8N_SEGREDO_ACOES", ""),

		VinculosSlackGitHub: leitura.vinculosSlackGitHub("SLACK_GITHUB_VINCULOS"),
	}

	if err := leitura.erro(); err != nil {
		return Configuracao{}, err
	}

	anunciarObrigatoriasFuturas()

	return configuracao, nil
}

// ConfigurarLogPadrao instala como logger global um handler JSON no nível
// definido pela configuração.
func ConfigurarLogPadrao(configuracao Configuracao) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuracao.NivelSlog(),
	})))
}

// NivelSlog traduz o nível de log configurado para o tipo usado por log/slog.
func (c Configuracao) NivelSlog() slog.Level {
	return niveisDeLogPorNome[c.NivelLog]
}

// EnderecoHTTP devolve o endereço de escuta do servidor no formato aceito por
// net/http, derivado da porta configurada.
func (c Configuracao) EnderecoHTTP() string {
	return net.JoinHostPort("", c.Porta)
}

type leitor struct {
	pendencias []string
}

func (l *leitor) obrigatoria(nome, ondeObter string) string {
	valor := lerVariavel(nome)
	if valor == "" {
		l.registrarPendencia("%s não definida (%s)", nome, ondeObter)
	}
	return valor
}

func (l *leitor) opcional(nome, padrao string) string {
	valor := lerVariavel(nome)
	if valor == "" {
		slog.Debug("variável opcional ausente", "variavel", nome, "padrao", padrao)
		return padrao
	}
	return valor
}

func (l *leitor) base64Opcional(nome string) []byte {
	valor := l.opcional(nome, "")
	if valor == "" {
		return nil
	}

	decodificado, err := base64.StdEncoding.DecodeString(valor)
	if err != nil {
		l.registrarPendencia("%s não está em base64 válido", nome)
		return nil
	}
	return decodificado
}

func (l *leitor) chavePrivadaEd25519(nome string) ed25519.PrivateKey {
	bruto := l.chaveComTamanho(nome, ed25519.PrivateKeySize)
	if bruto == nil {
		return nil
	}
	return ed25519.PrivateKey(bruto)
}

func (l *leitor) chavePublicaEd25519(nome string) ed25519.PublicKey {
	bruto := l.chaveComTamanho(nome, ed25519.PublicKeySize)
	if bruto == nil {
		return nil
	}
	return ed25519.PublicKey(bruto)
}

func (l *leitor) chaveComTamanho(nome string, tamanhoEsperado int) []byte {
	bruto := l.base64Opcional(nome)
	if bruto == nil {
		return nil
	}

	if len(bruto) != tamanhoEsperado {
		l.registrarPendencia("%s deve ter %d bytes decodificados, tem %d", nome, tamanhoEsperado, len(bruto))
		return nil
	}
	return bruto
}

func (l *leitor) vinculosSlackGitHub(nome string) map[string]string {
	vinculos := map[string]string{}

	valor := l.opcional(nome, "")
	if valor == "" {
		return vinculos
	}

	for _, entrada := range strings.Split(valor, ",") {
		usuario, login, valido := interpretarVinculo(entrada)
		if !valido {
			l.registrarPendencia("%s espera pares usuario_slack:login_github separados por vírgula, entrada %q é inválida", nome, strings.TrimSpace(entrada))
			return nil
		}
		if _, repetido := vinculos[usuario]; repetido {
			l.registrarPendencia("%s vincula o usuário %q mais de uma vez", nome, usuario)
			return nil
		}
		vinculos[usuario] = login
	}

	return vinculos
}

func interpretarVinculo(entrada string) (string, string, bool) {
	usuario, login, separado := strings.Cut(strings.TrimSpace(entrada), ":")
	usuario = strings.TrimSpace(usuario)
	login = strings.TrimSpace(login)

	valido := separado && usuario != "" && login != "" && !strings.Contains(login, ":")
	return usuario, login, valido
}

func (l *leitor) nivelDeLog() string {
	valor := l.opcional("LOG_LEVEL", nivelLogPadrao)
	if _, aceito := niveisDeLogPorNome[valor]; !aceito {
		l.registrarPendencia("LOG_LEVEL aceita %s, obtive %q",
			strings.Join(nomesDosNiveisDeLog, ", "), valor)
	}
	return valor
}

func (l *leitor) registrarPendencia(formato string, argumentos ...any) {
	l.pendencias = append(l.pendencias, fmt.Sprintf(formato, argumentos...))
}

func (l *leitor) erro() error {
	if len(l.pendencias) == 0 {
		return nil
	}
	return fmt.Errorf("configuração inválida: %s", strings.Join(l.pendencias, "; "))
}

func anunciarObrigatoriasFuturas() {
	for _, variavel := range variaveisQueSeraoObrigatorias {
		if lerVariavel(variavel) == "" {
			slog.Debug("ainda opcional, será obrigatória quando a funcionalidade existir", "variavel", variavel)
		}
	}
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
