package sandbox

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

type chamadaDeComando struct {
	binario    string
	argumentos []string
}

type comandoFalso struct {
	mutex     sync.Mutex
	chamadas  []chamadaDeComando
	responder func(ctx context.Context, argumentos []string) (resultadoComando, error)
}

func (c *comandoFalso) executar(ctx context.Context, binario string, argumentos ...string) (resultadoComando, error) {
	c.mutex.Lock()
	c.chamadas = append(c.chamadas, chamadaDeComando{binario: binario, argumentos: argumentos})
	c.mutex.Unlock()

	return c.responder(ctx, argumentos)
}

func (c *comandoFalso) chamadasRegistradas() []chamadaDeComando {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return slices.Clone(c.chamadas)
}

func (c *comandoFalso) argumentosDoRun(t *testing.T) []string {
	t.Helper()

	for _, chamada := range c.chamadasRegistradas() {
		if len(chamada.argumentos) > 0 && chamada.argumentos[0] == "run" {
			return chamada.argumentos
		}
	}
	t.Fatal("nenhum docker run foi executado")
	return nil
}

func (c *comandoFalso) recebeu(argumentosEsperados ...string) bool {
	for _, chamada := range c.chamadasRegistradas() {
		if slices.Equal(chamada.argumentos, argumentosEsperados) {
			return true
		}
	}
	return false
}

func valorDaFlag(argumentos []string, flag string) string {
	for indice, argumento := range argumentos {
		if argumento == flag && indice+1 < len(argumentos) {
			return argumentos[indice+1]
		}
	}
	return ""
}

func pedidoValido() PedidoExecucao {
	return PedidoExecucao{
		URLRepositorio: "https://github.com/Azulpasta/aPRova.git",
		SHA:            "3f2c1e0b9a8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b",
		Linguagem:      LinguagemGo,
		TaskRunID:      "task-run-1",
	}
}

func opcoesLocaisDeTeste() OpcoesExecutorLocal {
	return OpcoesExecutorLocal{
		ImagemPorLinguagem: map[Linguagem]string{LinguagemGo: "imagem-de-teste"},
	}
}

func TestExecucaoQueExcedeOPrazoEFalhaDeInfraestruturaENaoDeTeste(t *testing.T) {
	comando := &comandoFalso{responder: func(ctx context.Context, argumentos []string) (resultadoComando, error) {
		if argumentos[0] == "run" {
			<-ctx.Done()
			return resultadoComando{}, ctx.Err()
		}
		return resultadoComando{}, nil
	}}
	executor := novoExecutorLocal(opcoesLocaisDeTeste(), comando)

	ctx, cancelar := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancelar()

	resultado, err := executor.Executar(ctx, pedidoValido())

	if err != nil {
		t.Fatalf("timeout é um resultado classificado, não um erro de chamada: %v", err)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("timeout é culpa da infraestrutura, nunca do PR; esperava %q, obtive %q",
			FalhaInfraestrutura, resultado.TipoFalha)
	}

	nomeDoContainer := valorDaFlag(comando.argumentosDoRun(t), "--name")
	if nomeDoContainer == "" {
		t.Fatal("o container precisa de nome explícito, senão não há como removê-lo depois")
	}
	if !comando.recebeu("rm", "-f", nomeDoContainer) {
		t.Errorf("matar o docker run não mata o container; esperava rm -f %s, chamadas: %v",
			nomeDoContainer, comando.chamadasRegistradas())
	}
}
