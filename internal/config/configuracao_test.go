package config

import (
	"reflect"
	"strings"
	"testing"
)

func definirVariaveisValidas(t *testing.T) {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgres://usuario:senha@localhost:5432/aprova")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("PORT", "8080")
	t.Setenv("GITHUB_APP_ID", "123456")
	t.Setenv("GITHUB_PRIVATE_KEY", "conteudo-da-chave")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "segredo-do-webhook")
	t.Setenv("RECEIPT_PRIVATE_KEY", "chave-do-recibo")
}

func TestCarregarRejeitaVariavelPreenchidaSoComEspacos(t *testing.T) {
	definirVariaveisValidas(t)
	t.Setenv("DATABASE_URL", "   ")

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro para DATABASE_URL preenchida só com espaços, obtive nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("o erro deveria nomear DATABASE_URL, obtive: %v", err)
	}
}

func TestNomeDaVariavelTraduzCampoMapeado(t *testing.T) {
	if nome := nomeDaVariavel("URLBancoDados"); nome != "DATABASE_URL" {
		t.Errorf("esperava DATABASE_URL, obtive %q", nome)
	}
}

func TestNomeDaVariavelUsaOCampoQuandoNaoEstaMapeado(t *testing.T) {
	const campoNovo = "CampoAindaNaoMapeado"

	if nome := nomeDaVariavel(campoNovo); nome != campoNovo {
		t.Errorf("para campo não mapeado esperava o fallback %q, obtive %q", campoNovo, nome)
	}
}

func TestCarregarListaTodasAsVariaveisAusentes(t *testing.T) {
	for _, variavel := range variavelDeAmbientePorCampo {
		t.Setenv(variavel, "")
	}

	_, err := Carregar()

	if err == nil {
		t.Fatal("esperava erro com todas as variáveis ausentes, obtive nil")
	}
	for campo, variavel := range variavelDeAmbientePorCampo {
		if variavel == "PORT" {
			continue
		}
		if !strings.Contains(err.Error(), variavel) {
			t.Errorf("o erro deveria nomear %s (campo %s), obtive: %v", variavel, campo, err)
		}
	}
}

func TestCarregarUsaPortaPadraoQuandoAusente(t *testing.T) {
	definirVariaveisValidas(t)
	t.Setenv("PORT", "")

	configuracao, err := Carregar()

	if err != nil {
		t.Fatalf("PORT ausente deve cair no padrão, não falhar: %v", err)
	}
	if configuracao.Porta != portaPadrao {
		t.Errorf("esperava porta padrão %q, obtive %q", portaPadrao, configuracao.Porta)
	}
}

func TestCarregarRejeitaPortaNaoNumerica(t *testing.T) {
	definirVariaveisValidas(t)
	t.Setenv("PORT", "oitenta")

	if _, err := Carregar(); err == nil {
		t.Fatal("esperava erro para PORT não numérica, obtive nil")
	}
}

func TestEnderecoHTTPEscutaEmTodasAsInterfaces(t *testing.T) {
	if endereco := (Configuracao{Porta: "9000"}).EnderecoHTTP(); endereco != ":9000" {
		t.Errorf("esperava \":9000\", obtive %q", endereco)
	}
}

func TestTodoCampoDeConfiguracaoTemVariavelMapeada(t *testing.T) {
	tipo := reflect.TypeOf(Configuracao{})

	for indice := 0; indice < tipo.NumField(); indice++ {
		campo := tipo.Field(indice).Name
		if _, mapeado := variavelDeAmbientePorCampo[campo]; !mapeado {
			t.Errorf("o campo %s não tem variável de ambiente mapeada", campo)
		}
	}
}
