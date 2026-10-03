package sandbox

import "context"

// Linguagem identifica a linguagem do repositório a ser testado.
type Linguagem string

// Linguagens aceitas pelo sandbox.
const (
	LinguagemGo     Linguagem = "go"
	LinguagemPython Linguagem = "python"
	LinguagemJava   Linguagem = "java"
)

// TipoFalha classifica de quem é a culpa quando a execução não termina bem.
type TipoFalha string

// Tipos de falha possíveis em um ResultadoSandbox.
const (
	FalhaNenhuma        TipoFalha = "nenhuma"
	FalhaTeste          TipoFalha = "teste"
	FalhaCompilacao     TipoFalha = "compilacao"
	FalhaInfraestrutura TipoFalha = "infraestrutura"
)

// PedidoExecucao descreve o que deve ser testado no sandbox.
type PedidoExecucao struct {
	URLRepositorio string    `validate:"required,url"`
	SHA            string    `validate:"required,hexadecimal,min=40,max=64"`
	Linguagem      Linguagem `validate:"required,oneof=go python java"`
	TaskRunID      string    `validate:"required"`
}

// ResultadoSandbox é o resultado normalizado de uma execução, igual para
// qualquer linguagem e qualquer executor.
type ResultadoSandbox struct {
	Compilou       bool      `json:"compilou"`
	TestesPassaram int       `json:"testes_passaram"`
	TestesFalharam int       `json:"testes_falharam"`
	SaidaBruta     string    `json:"saida_bruta"`
	DuracaoMS      int64     `json:"duracao_ms"`
	TipoFalha      TipoFalha `json:"tipo_falha"`
}

// Executor executa um pedido em ambiente isolado. Falhas de infraestrutura
// voltam como ResultadoSandbox classificado; o erro fica reservado para pedido
// inválido.
type Executor interface {
	Executar(ctx context.Context, pedido PedidoExecucao) (ResultadoSandbox, error)
}

// ColetorResultado recebe o resultado gravado pelo job e o entrega a quem
// aguarda por aquele task_run.
type ColetorResultado interface {
	Gravar(ctx context.Context, taskRunID string, resultado ResultadoSandbox) error
	Aguardar(ctx context.Context, taskRunID string) (ResultadoSandbox, error)
}
