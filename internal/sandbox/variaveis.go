package sandbox

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
