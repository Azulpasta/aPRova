package sandbox

import (
	"context"
	"errors"
	"testing"
)

func comandoQueNaoDeveSerChamado(t *testing.T) *comandoFalso {
	t.Helper()

	return &comandoFalso{responder: func(_ context.Context, argumentos []string) (resultadoComando, error) {
		t.Errorf("pedido inválido não pode executar nada; recebi docker %v", argumentos)
		return resultadoComando{}, nil
	}}
}

func TestPedidoInvalidoERejeitadoSemExecutarNada(t *testing.T) {
	casos := map[string]func(*PedidoExecucao){
		"linguagem desconhecida": func(p *PedidoExecucao) { p.Linguagem = "cobol" },
		"linguagem ausente":      func(p *PedidoExecucao) { p.Linguagem = "" },
		"sha ausente":            func(p *PedidoExecucao) { p.SHA = "" },
		"sha que não é hash":     func(p *PedidoExecucao) { p.SHA = "main; rm -rf /" },
		"url que não é url":      func(p *PedidoExecucao) { p.URLRepositorio = "nao-e-url" },
		"task run ausente":       func(p *PedidoExecucao) { p.TaskRunID = "" },
	}

	for nome, estragar := range casos {
		t.Run(nome, func(t *testing.T) {
			comando := comandoQueNaoDeveSerChamado(t)
			executor := novoExecutorLocal(opcoesLocaisDeTeste(), comando)

			pedido := pedidoValido()
			estragar(&pedido)

			_, err := executor.Executar(t.Context(), pedido)

			if !errors.Is(err, ErrPedidoInvalido) {
				t.Errorf("esperava ErrPedidoInvalido, obtive %v", err)
			}
			if chamadas := comando.chamadasRegistradas(); len(chamadas) != 0 {
				t.Errorf("nenhum container pode subir para pedido inválido; houve %d chamada(s)", len(chamadas))
			}
		})
	}
}
