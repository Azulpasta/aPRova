package sandbox

import (
	"context"
	"strconv"
	"time"
)

const (
	prazoPadraoDoContainer    = 10 * time.Minute
	margemParaSubirOContainer = 15 * time.Second
)

type variavelDeAmbiente struct {
	nome  string
	valor string
}

func variaveisDoPedido(pedido PedidoExecucao) []variavelDeAmbiente {
	return []variavelDeAmbiente{
		{nome: "APROVA_REPOSITORIO", valor: pedido.URLRepositorio},
		{nome: "APROVA_SHA", valor: pedido.SHA},
		{nome: "APROVA_LINGUAGEM", valor: string(pedido.Linguagem)},
		{nome: "APROVA_TASK_RUN_ID", valor: pedido.TaskRunID},
	}
}

func variaveisDoContainer(ctx context.Context, pedido PedidoExecucao, destinoDoResultado string) []variavelDeAmbiente {
	return append(variaveisDoPedido(pedido),
		variavelDeAmbiente{nome: "APROVA_PRAZO_SEGUNDOS", valor: strconv.Itoa(prazoEmSegundos(ctx))},
		variavelDeAmbiente{nome: "APROVA_SAIDA", valor: destinoDoResultado},
	)
}

func prazoEmSegundos(ctx context.Context) int {
	limite, definido := ctx.Deadline()
	if !definido {
		return int(prazoPadraoDoContainer.Seconds())
	}
	return max(1, int((time.Until(limite) - margemParaSubirOContainer).Seconds()))
}
