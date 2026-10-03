package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"
	"strings"
	"testing"
)

func ambienteLimpo(t *testing.T) {
	t.Helper()

	for _, variavel := range []string{
		"PORT", "LOG_LEVEL", "DATABASE_URL", "REDIS_URL",
		"GITHUB_APP_ID", "GITHUB_PRIVATE_KEY_BASE64", "GITHUB_WEBHOOK_SECRET",
		"RECEIPT_PRIVATE_KEY_BASE64", "RECEIPT_PUBLIC_KEY_BASE64",
		"ANTHROPIC_API_KEY", "GCP_PROJECT_ID", "GCP_REGION",
		"SANDBOX_EXECUTOR", "N8N_WEBHOOK_URL",
		"AZURE_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET", "AZURE_RESOURCE_GROUP", "AZURE_SANDBOX_JOB_NAME",
	} {
		t.Setenv(variavel, "")
	}
}

func ambienteMinimoValido(t *testing.T) {
	t.Helper()

	ambienteLimpo(t)
	t.Setenv("DATABASE_URL", "postgres://aprova:aprova@localhost:5432/aprova?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
}

func capturarLogDebug(t *testing.T) *bytes.Buffer {
	t.Helper()

	var registro bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	return &registro
}

func TestCarregarFalhaQuandoVariavelObrigatoriaEstaAusente(t *testing.T) {
	ambienteMinimoValido(t)
	t.Setenv("DATABASE_URL", "")

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro com DATABASE_URL ausente, obtive nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("o erro deve nomear a variável, obtive: %v", err)
	}
	if !strings.Contains(err.Error(), "docker-compose") {
		t.Errorf("o erro deve dizer onde obter o valor, obtive: %v", err)
	}
}

func TestCarregarNaoExigeVariaveisAindaNaoUsadas(t *testing.T) {
	ambienteMinimoValido(t)

	if _, err := Carregar(); err != nil {
		t.Fatalf("GitHub, agente e recibo são opcionais nesta etapa: %v", err)
	}
}

func TestCarregarAplicaValoresPadrao(t *testing.T) {
	ambienteMinimoValido(t)

	configuracao, err := Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	for _, caso := range []struct {
		nome     string
		obtido   string
		esperado string
	}{
		{"PORT", configuracao.Porta, portaPadrao},
		{"LOG_LEVEL", configuracao.NivelLog, nivelLogPadrao},
		{"GCP_REGION", configuracao.GCPRegiao, regiaoPadrao},
	} {
		if caso.obtido != caso.esperado {
			t.Errorf("%s ausente deveria cair no padrão %q, obtive %q", caso.nome, caso.esperado, caso.obtido)
		}
	}
}

func TestCarregarRegistraEmDebugCadaOpcionalAusente(t *testing.T) {
	registro := capturarLogDebug(t)
	ambienteMinimoValido(t)

	if _, err := Carregar(); err != nil {
		t.Fatalf("carregar: %v", err)
	}

	if !strings.Contains(registro.String(), "ANTHROPIC_API_KEY") {
		t.Errorf("esperava log debug citando a opcional ausente, obtive: %s", registro.String())
	}
}

func TestCarregarRepassaOExecutorDoSandboxSemPadraoNemValidacao(t *testing.T) {
	for _, valor := range []string{"", "azure", "local", "valor-que-o-sandbox-vai-recusar"} {
		t.Run("SANDBOX_EXECUTOR="+valor, func(t *testing.T) {
			ambienteMinimoValido(t)
			t.Setenv("SANDBOX_EXECUTOR", valor)

			configuracao, err := Carregar()
			if err != nil {
				t.Fatalf("validar o executor é papel do pacote sandbox, não do config: %v", err)
			}
			if configuracao.SandboxExecutor != valor {
				t.Errorf("esperava o valor cru %q, obtive %q", valor, configuracao.SandboxExecutor)
			}
		})
	}
}

func TestCarregarLeAsCredenciaisDaAzure(t *testing.T) {
	ambienteMinimoValido(t)
	esperadas := map[string]string{
		"AZURE_SUBSCRIPTION_ID":  "assinatura",
		"AZURE_TENANT_ID":        "tenant",
		"AZURE_CLIENT_ID":        "cliente",
		"AZURE_CLIENT_SECRET":    "segredo",
		"AZURE_RESOURCE_GROUP":   "grupo",
		"AZURE_SANDBOX_JOB_NAME": "job",
	}
	for nome, valor := range esperadas {
		t.Setenv(nome, valor)
	}

	configuracao, err := Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	obtidas := map[string]string{
		"AZURE_SUBSCRIPTION_ID":  configuracao.AzureSubscriptionID,
		"AZURE_TENANT_ID":        configuracao.AzureTenantID,
		"AZURE_CLIENT_ID":        configuracao.AzureClientID,
		"AZURE_CLIENT_SECRET":    configuracao.AzureClientSecret,
		"AZURE_RESOURCE_GROUP":   configuracao.AzureResourceGroup,
		"AZURE_SANDBOX_JOB_NAME": configuracao.AzureNomeDoJobSandbox,
	}
	for nome, valor := range esperadas {
		if obtidas[nome] != valor {
			t.Errorf("%s deveria ser %q, obtive %q", nome, valor, obtidas[nome])
		}
	}
}

func TestCarregarRejeitaChaveDeReciboQueNaoEBase64(t *testing.T) {
	ambienteMinimoValido(t)
	t.Setenv("RECEIPT_PRIVATE_KEY_BASE64", "isto não é base64!!!")

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro na carga, não no primeiro uso")
	}
	if !strings.Contains(err.Error(), "RECEIPT_PRIVATE_KEY_BASE64") {
		t.Errorf("o erro deve nomear a variável, obtive: %v", err)
	}
}

func TestCarregarRejeitaChaveDeReciboComTamanhoErrado(t *testing.T) {
	ambienteMinimoValido(t)
	t.Setenv("RECEIPT_PRIVATE_KEY_BASE64", base64.StdEncoding.EncodeToString([]byte("curta demais")))

	if _, err := Carregar(); err == nil {
		t.Fatal("esperava erro para chave Ed25519 de tamanho inválido")
	}
}

func TestCarregarDecodificaParDeChavesUsavelParaAssinar(t *testing.T) {
	publica, privada, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("gerar par: %v", err)
	}

	ambienteMinimoValido(t)
	t.Setenv("RECEIPT_PRIVATE_KEY_BASE64", base64.StdEncoding.EncodeToString(privada))
	t.Setenv("RECEIPT_PUBLIC_KEY_BASE64", base64.StdEncoding.EncodeToString(publica))

	configuracao, err := Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	mensagem := []byte("recibo de exemplo")
	assinatura := ed25519.Sign(configuracao.ReciboChavePrivada, mensagem)

	if !ed25519.Verify(configuracao.ReciboChavePublica, mensagem, assinatura) {
		t.Error("a chave pública carregada deveria verificar o que a privada carregada assinou")
	}
}

func TestCarregarDecodificaChavePrivadaDoGitHub(t *testing.T) {
	const pem = "-----BEGIN RSA PRIVATE KEY-----\nconteudo\n-----END RSA PRIVATE KEY-----"

	ambienteMinimoValido(t)
	t.Setenv("GITHUB_PRIVATE_KEY_BASE64", base64.StdEncoding.EncodeToString([]byte(pem)))

	configuracao, err := Carregar()
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	if string(configuracao.GitHubChavePrivada) != pem {
		t.Errorf("esperava o PEM decodificado, obtive %q", configuracao.GitHubChavePrivada)
	}
}

func TestEnderecoHTTPEscutaEmTodasAsInterfaces(t *testing.T) {
	if endereco := (Configuracao{Porta: "9000"}).EnderecoHTTP(); endereco != ":9000" {
		t.Errorf("esperava \":9000\", obtive %q", endereco)
	}
}

func TestNivelSlogTraduzOsNiveisAceitos(t *testing.T) {
	for texto, esperado := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	} {
		if obtido := (Configuracao{NivelLog: texto}).NivelSlog(); obtido != esperado {
			t.Errorf("LOG_LEVEL=%s deveria virar %v, obtive %v", texto, esperado, obtido)
		}
	}
}

func TestCarregarRejeitaNivelDeLogDesconhecido(t *testing.T) {
	ambienteMinimoValido(t)
	t.Setenv("LOG_LEVEL", "verboso")

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro para LOG_LEVEL desconhecido, obtive nil")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("o erro deve nomear a variável, obtive: %v", err)
	}
}

func TestConfigurarLogPadraoRespeitaONivelConfigurado(t *testing.T) {
	anterior := slog.Default()
	t.Cleanup(func() { slog.SetDefault(anterior) })

	ConfigurarLogPadrao(Configuracao{NivelLog: "warn"})

	if slog.Default().Enabled(t.Context(), slog.LevelInfo) {
		t.Error("nível warn não deveria habilitar info")
	}
	if !slog.Default().Enabled(t.Context(), slog.LevelError) {
		t.Error("nível warn deveria habilitar error")
	}
}
