package sandbox

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func executarComSaida(t *testing.T, saida string, limite int) ResultadoSandbox {
	t.Helper()

	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{saida: saida}, nil
	}}
	opcoes := opcoesLocaisDeTeste()
	opcoes.LimiteSaidaBytes = limite

	resultado, err := novoExecutorLocal(opcoes, comando).Executar(t.Context(), pedidoValido())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}
	return resultado
}

func TestSaidaMaiorQueOLimiteETruncadaMantendoOFinal(t *testing.T) {
	const ondeAFalhaAparece = "--- FAIL: TestCritico"
	saida := strings.Repeat("log irrelevante do começo\n", 200) + ondeAFalhaAparece

	resultado := executarComSaida(t, saida, 64)

	if len(resultado.SaidaBruta) > 64 {
		t.Errorf("a saída deveria caber em 64 bytes, tem %d", len(resultado.SaidaBruta))
	}
	if !strings.HasSuffix(resultado.SaidaBruta, ondeAFalhaAparece) {
		t.Errorf("o final é onde a falha costuma estar e precisa sobreviver; obtive %q", resultado.SaidaBruta)
	}
}

func TestSaidaDentroDoLimiteFicaIntacta(t *testing.T) {
	const saida = "ok  github.com/Azulpasta/aPRova 0.01s"

	if resultado := executarComSaida(t, saida, 1024); resultado.SaidaBruta != saida {
		t.Errorf("saída curta não deveria mudar; obtive %q", resultado.SaidaBruta)
	}
}

func TestTruncamentoNaoPartilhaCaractereMultibyte(t *testing.T) {
	saida := strings.Repeat("ação ", 50)

	for limite := 1; limite <= 12; limite++ {
		resultado := executarComSaida(t, saida, limite)

		if !utf8.ValidString(resultado.SaidaBruta) {
			t.Errorf("com limite %d o corte partiu um caractere: %q", limite, resultado.SaidaBruta)
		}
		if len(resultado.SaidaBruta) > limite {
			t.Errorf("com limite %d a saída tem %d bytes", limite, len(resultado.SaidaBruta))
		}
	}
}
