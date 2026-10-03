package sandbox

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func executarComCodigo(t *testing.T, codigo int) ResultadoSandbox {
	t.Helper()

	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{saida: "saída qualquer", codigo: codigo}, nil
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}
	return resultado
}

func TestCodigoDeSaidaEClassificadoPorQuemTemACulpa(t *testing.T) {
	casos := []struct {
		descricao        string
		codigo           int
		tipoEsperado     TipoFalha
		compilouEsperado bool
	}{
		{"tudo passou", 0, FalhaNenhuma, true},
		{"testes do PR falharam", codigoFalhaDeTeste, FalhaTeste, true},
		{"código do PR não compila", codigoFalhaDeCompilacao, FalhaCompilacao, false},
		{"docker não subiu o container, imagem ausente", 125, FalhaInfraestrutura, false},
		{"comando do container não pôde ser invocado", 126, FalhaInfraestrutura, false},
		{"comando do container não encontrado", 127, FalhaInfraestrutura, false},
		{"container morto por sinal ou falta de memória", 137, FalhaInfraestrutura, false},
	}

	for _, caso := range casos {
		t.Run(caso.descricao, func(t *testing.T) {
			resultado := executarComCodigo(t, caso.codigo)

			if resultado.TipoFalha != caso.tipoEsperado {
				t.Errorf("código %d: esperava %q, obtive %q", caso.codigo, caso.tipoEsperado, resultado.TipoFalha)
			}
			if resultado.Compilou != caso.compilouEsperado {
				t.Errorf("código %d: compilou deveria ser %v", caso.codigo, caso.compilouEsperado)
			}
		})
	}
}

func TestDockerAusenteEFalhaDeInfraestrutura(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, errors.New(`exec: "docker": executable file not found in $PATH`)
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())

	if err != nil {
		t.Fatalf("falha de infraestrutura é resultado, não erro de chamada: %v", err)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("sem docker a culpa é nossa; esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
}

func TestContainerSobeIsoladoELimitado(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}

	if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido()); err != nil {
		t.Fatalf("executar: %v", err)
	}
	argumentos := comando.argumentosDoRun(t)

	for _, obrigatorio := range []string{"--rm", "--read-only"} {
		if !slices.Contains(argumentos, obrigatorio) {
			t.Errorf("faltou %s nos argumentos: %v", obrigatorio, argumentos)
		}
	}

	exigencias := map[string]string{
		"--user":         usuarioSemPrivilegio,
		"--network":      redePadrao,
		"--cpus":         limiteCPUPadrao,
		"--memory":       limiteMemoriaPadrao,
		"--pids-limit":   limiteProcessosPadrao,
		"--cap-drop":     "ALL",
		"--security-opt": "no-new-privileges",
		"--tmpfs":        "/tmp",
	}
	for flag, esperado := range exigencias {
		if obtido := valorDaFlag(argumentos, flag); obtido != esperado {
			t.Errorf("%s deveria ser %q, obtive %q", flag, esperado, obtido)
		}
	}
	if valorDaFlag(argumentos, "--user") == "0" || valorDaFlag(argumentos, "--user") == "root" {
		t.Error("o código do PR nunca roda como root")
	}
}

func TestContainerRecebeOPedidoPorVariaveisDeAmbiente(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}
	pedido := pedidoValido()

	if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido); err != nil {
		t.Fatalf("executar: %v", err)
	}
	argumentos := comando.argumentosDoRun(t)

	for _, esperado := range []string{
		"APROVA_REPOSITORIO=" + pedido.URLRepositorio,
		"APROVA_SHA=" + pedido.SHA,
		"APROVA_LINGUAGEM=" + string(pedido.Linguagem),
		"APROVA_TASK_RUN_ID=" + pedido.TaskRunID,
	} {
		if !contemVariavel(argumentos, esperado) {
			t.Errorf("faltou -e %s nos argumentos: %v", esperado, argumentos)
		}
	}

	if imagem := argumentos[len(argumentos)-1]; imagem != "imagem-de-teste" {
		t.Errorf("a imagem da linguagem deveria ser o último argumento, obtive %q", imagem)
	}
}

func TestDuracaoDaExecucaoEMedida(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		time.Sleep(20 * time.Millisecond)
		return resultadoComando{}, nil
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}
	if resultado.DuracaoMS < 20 {
		t.Errorf("a execução levou ao menos 20ms, registrou %dms", resultado.DuracaoMS)
	}
}

func TestLinguagemSemImagemConfiguradaEFalhaDeInfraestruturaSemChamarDocker(t *testing.T) {
	comando := comandoQueNaoDeveSerChamado(t)
	pedido := pedidoValido()
	pedido.Linguagem = LinguagemPython

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido)

	if err != nil {
		t.Fatalf("imagem não configurada é problema nosso, resultado e não erro: %v", err)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
}

func contemVariavel(argumentos []string, atribuicao string) bool {
	for indice, argumento := range argumentos {
		if argumento == "-e" && indice+1 < len(argumentos) && argumentos[indice+1] == atribuicao {
			return true
		}
	}
	return false
}
