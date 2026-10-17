package processo

import "context"

// Comando descreve um processo a executar: binário, argumentos, diretório de
// trabalho e o ambiente completo, sem herdar o do processo atual.
type Comando struct {
	Binario    string
	Argumentos []string
	Diretorio  string
	Ambiente   []string
}

// Execucao é o desfecho de um comando. Saida junta stdout e stderr na ordem
// em que chegaram. Erro indica que o processo nem pôde ser iniciado.
type Execucao struct {
	Saida         []byte
	CodigoSaida   int
	PrazoEsgotado bool
	Erro          error
}

// Rodador executa comandos e captura a saída deles.
type Rodador interface {
	Rodar(ctx context.Context, comando Comando) Execucao
}
