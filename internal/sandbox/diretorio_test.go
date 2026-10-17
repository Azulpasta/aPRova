package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidarDiretorioDoProjetoAceitaCaminhosRelativosDentroDoRepositorio(t *testing.T) {
	for _, diretorio := range []string{"", "go", "servicos/frete", "go-1.2_x"} {
		if err := ValidarDiretorioDoProjeto(diretorio); err != nil {
			t.Errorf("%q é um diretório de projeto válido: %v", diretorio, err)
		}
	}
}

func TestValidarDiretorioDoProjetoRecusaQuemSaiDoRepositorioOuEAmbiguo(t *testing.T) {
	for _, diretorio := range []string{
		"/go", "..", "../outro", "go/../..", "go/../../etc", "./go", "go/", "go//frete", ".", "go/./frete", "go\x00", " go",
	} {
		if err := ValidarDiretorioDoProjeto(diretorio); !errors.Is(err, ErrDiretorioInvalido) {
			t.Errorf("%q precisa ser recusado com ErrDiretorioInvalido, obtive %v", diretorio, err)
		}
	}
}

func TestPedidoComDiretorioInvalidoERejeitadoSemExecutarNada(t *testing.T) {
	comando := comandoQueNaoDeveSerChamado(t)
	pedido := pedidoValido()
	pedido.Diretorio = "../fora"

	_, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido)

	if !errors.Is(err, ErrPedidoInvalido) {
		t.Errorf("esperava ErrPedidoInvalido, obtive %v", err)
	}
}

func TestDiretorioDoPedidoChegaAoContainerLocal(t *testing.T) {
	comando := &comandoFalso{responder: func(context.Context, []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}
	pedido := pedidoValido()
	pedido.Diretorio = "go"

	if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido); err != nil {
		t.Fatalf("executar: %v", err)
	}

	if argumentos := comando.argumentosDoRun(t); !contemVariavel(argumentos, "APROVA_DIRETORIO=go") {
		t.Errorf("o container precisa receber APROVA_DIRETORIO: %v", argumentos)
	}
}

func TestDiretorioDoPedidoChegaAoJobDaAzure(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Succeeded"}}
	coletor := NovoColetorEmMemoria()
	_ = coletor.Gravar(t.Context(), pedidoValido().TaskRunID, resultadoDeExemplo())
	pedido := pedidoValido()
	pedido.Diretorio = "go"

	ctx, cancelar := context.WithTimeout(t.Context(), time.Second)
	defer cancelar()
	if _, err := novoExecutorAzure(opcoesAzureDeTeste(), cliente, coletor).Executar(ctx, pedido); err != nil {
		t.Fatalf("executar: %v", err)
	}

	if diretorio := cliente.variaveisRecebidas["APROVA_DIRETORIO"]; diretorio != "go" {
		t.Errorf("APROVA_DIRETORIO = %q, esperava go", diretorio)
	}
}
