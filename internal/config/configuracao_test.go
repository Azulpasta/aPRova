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
		{"SANDBOX_EXECUTOR", configuracao.SandboxExecutor, executorPadrao},
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

func TestCarregarRejeitaExecutorDeSandboxDesconhecido(t *testing.T) {
	ambienteMinimoValido(t)
	t.Setenv("SANDBOX_EXECUTOR", "kubernetes")

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro para executor desconhecido, obtive nil")
	}
	if !strings.Contains(err.Error(), "SANDBOX_EXECUTOR") {
		t.Errorf("o erro deve nomear a variável, obtive: %v", err)
	}
}

func TestCarregarAceitaOsTresExecutoresPrevistos(t *testing.T) {
	for _, executor := range []string{"local", "cloudrun", "actions"} {
		t.Run(executor, func(t *testing.T) {
			ambienteMinimoValido(t)
			t.Setenv("SANDBOX_EXECUTOR", executor)

			configuracao, err := Carregar()
			if err != nil {
				t.Fatalf("executor %q deveria ser aceito: %v", executor, err)
			}
			if configuracao.SandboxExecutor != executor {
				t.Errorf("esperava %q, obtive %q", executor, configuracao.SandboxExecutor)
			}
		})
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
