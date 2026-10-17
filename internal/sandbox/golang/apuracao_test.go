package golang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lerGravacao(t *testing.T, nome string) []byte {
	t.Helper()

	conteudo, err := os.ReadFile(filepath.Join("testdata", nome))
	if err != nil {
		t.Fatalf("ler gravação %s: %v", nome, err)
	}
	return conteudo
}

func TestApurarContaTestesDeNivelSuperior(t *testing.T) {
	casos := []struct {
		gravacao          string
		passaram          int
		falharam          int
		falhaDeCompilacao bool
	}{
		{"so_passando.jsonl", 3, 0, false},
		{"um_falho.jsonl", 2, 1, false},
		{"subtestes.jsonl", 1, 1, false},
		{"teste_nao_compila.jsonl", 0, 0, true},
		{"panic_em_testmain.jsonl", 0, 1, false},
		{"pacote_sem_testes.jsonl", 1, 0, false},
		{"teste_que_trava.jsonl", 1, 1, false},
		{"linha_invalida.jsonl", 2, 1, false},
	}

	for _, caso := range casos {
		t.Run(caso.gravacao, func(t *testing.T) {
			apurada := apurar(lerGravacao(t, caso.gravacao))

			if apurada.passaram != caso.passaram || apurada.falharam != caso.falharam {
				t.Errorf("passaram %d e falharam %d, esperava %d e %d",
					apurada.passaram, apurada.falharam, caso.passaram, caso.falharam)
			}
			if apurada.falhaDeCompilacao != caso.falhaDeCompilacao {
				t.Errorf("falha de compilação = %v, esperava %v", apurada.falhaDeCompilacao, caso.falhaDeCompilacao)
			}
		})
	}
}

func TestApurarJuntaASaidaDosEventosEmOrdem(t *testing.T) {
	saida := apurar(lerGravacao(t, "um_falho.jsonl")).saida

	ordem := []string{
		"=== RUN   TestSomaPositivos\n",
		"=== RUN   TestFrete\n",
		"frete: esperava 16, obtive 15",
		"--- FAIL: TestFrete",
		"FAIL\texemplo.local/umfalho",
	}
	posicaoAnterior := -1
	for _, trecho := range ordem {
		posicao := strings.Index(saida, trecho)
		if posicao <= posicaoAnterior {
			t.Fatalf("trecho %q ausente ou fora de ordem na saída:\n%s", trecho, saida)
		}
		posicaoAnterior = posicao
	}
}

func TestApurarIncluiASaidaDaCompilacaoDosTestes(t *testing.T) {
	saida := apurar(lerGravacao(t, "teste_nao_compila.jsonl")).saida

	if !strings.Contains(saida, `cannot use "texto"`) {
		t.Errorf("o erro de compilação precisa aparecer na saída bruta:\n%s", saida)
	}
}

func TestApurarPreservaLinhaInvalidaSemContaLa(t *testing.T) {
	apurada := apurar(lerGravacao(t, "linha_invalida.jsonl"))

	for _, linha := range []string{"isto não é json, veio do stderr\n", `{"Action":"pass","Test":"TestQuebrado"` + "\n"} {
		if !strings.Contains(apurada.saida, linha) {
			t.Errorf("a linha inválida %q precisa ficar na saída bruta", linha)
		}
	}
}

func TestApurarContaUmaFalhaPorPacoteQueFalhaSemTesteFalho(t *testing.T) {
	saida := `{"Action":"fail","Package":"exemplo.local/a"}
{"Action":"fail","Package":"exemplo.local/b"}
{"Action":"run","Package":"exemplo.local/c","Test":"TestX"}
{"Action":"fail","Package":"exemplo.local/c","Test":"TestX"}
{"Action":"fail","Package":"exemplo.local/c"}
`

	if falharam := apurar([]byte(saida)).falharam; falharam != 3 {
		t.Errorf("a e b falharam sem teste contado, c já tem o seu: esperava 3, obtive %d", falharam)
	}
}

func TestApurarIgnoraSubtesteMesmoQuandoSoEleFalha(t *testing.T) {
	saida := `{"Action":"fail","Package":"exemplo.local/a","Test":"TestPai/filho"}
{"Action":"pass","Package":"exemplo.local/a","Test":"TestPai/outro"}
`

	apurada := apurar([]byte(saida))

	if apurada.passaram != 0 || apurada.falharam != 0 {
		t.Errorf("subteste não entra na contagem, obtive %+v", apurada)
	}
}

func TestApurarSaidaVazia(t *testing.T) {
	apurada := apurar(nil)

	if apurada != (apuracao{}) {
		t.Errorf("saída vazia não tem nada a apurar, obtive %+v", apurada)
	}
}
