package conteiner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

var resultadoDeTeste = sandbox.ResultadoSandbox{
	Compilou:       true,
	TestesPassaram: 2,
	TestesFalharam: 1,
	SaidaBruta:     "linha um\nlinha dois\n",
	DuracaoMS:      1234,
	TipoFalha:      sandbox.FalhaTeste,
}

func TestPublicadorNaSaidaEscreveUmaUnicaLinhaJSON(t *testing.T) {
	var saida bytes.Buffer

	if err := NovoPublicadorNaSaida(&saida).Publicar(t.Context(), "task-run-1", resultadoDeTeste); err != nil {
		t.Fatalf("publicar não deveria falhar: %v", err)
	}

	if strings.Count(saida.String(), "\n") != 1 || !strings.HasSuffix(saida.String(), "\n") {
		t.Fatalf("esperava exatamente uma linha, obtive %q", saida.String())
	}
	var lido sandbox.ResultadoSandbox
	if err := json.Unmarshal(saida.Bytes(), &lido); err != nil || lido != resultadoDeTeste {
		t.Errorf("a linha precisa ser o resultado em JSON: %v, %+v", err, lido)
	}
}

type escritorQuebrado struct{}

func (escritorQuebrado) Write([]byte) (int, error) { return 0, errors.New("stdout fechado") }

func TestPublicadorNaSaidaDevolveErroDeEscrita(t *testing.T) {
	err := NovoPublicadorNaSaida(escritorQuebrado{}).Publicar(t.Context(), "task-run-1", resultadoDeTeste)

	if err == nil {
		t.Error("falha ao escrever precisa voltar como erro")
	}
}

func TestPublicadorNoColetorGravaPeloTaskRun(t *testing.T) {
	coletor := sandbox.NovoColetorEmMemoria()

	if err := NovoPublicadorNoColetor(coletor).Publicar(t.Context(), "task-run-1", resultadoDeTeste); err != nil {
		t.Fatalf("publicar não deveria falhar: %v", err)
	}

	ctx, cancelar := context.WithTimeout(t.Context(), time.Second)
	defer cancelar()
	gravado, err := coletor.Aguardar(ctx, "task-run-1")
	if err != nil || gravado != resultadoDeTeste {
		t.Errorf("o coletor precisa ter o resultado do task_run: %v, %+v", err, gravado)
	}
}

func TestEscolherPublicador(t *testing.T) {
	coletor := sandbox.NovoColetorEmMemoria()

	casos := []struct {
		destino  string
		coletor  sandbox.ColetorResultado
		esperado string
		erro     error
	}{
		{"", nil, "*conteiner.PublicadorNaSaida", nil},
		{"stdout", nil, "*conteiner.PublicadorNaSaida", nil},
		{"coletor", coletor, "*conteiner.PublicadorNoColetor", nil},
		{"coletor", nil, "", ErrColetorIndisponivel},
		{"arquivo", nil, "", ErrSaidaDesconhecida},
	}

	for _, caso := range casos {
		t.Run(caso.destino, func(t *testing.T) {
			publicador, err := EscolherPublicador(caso.destino, &bytes.Buffer{}, caso.coletor)

			if !errors.Is(err, caso.erro) {
				t.Fatalf("erro = %v, esperava %v", err, caso.erro)
			}
			if caso.erro == nil && fmt.Sprintf("%T", publicador) != caso.esperado {
				t.Errorf("publicador = %T, esperava %s", publicador, caso.esperado)
			}
		})
	}
}
