package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"
)

const (
	binarioDockerPadrao = "docker"
	tempoLimiteRemocao  = 15 * time.Second

	usuarioSemPrivilegio  = "65534:65534"
	redePadrao            = "none"
	limiteCPUPadrao       = "1"
	limiteMemoriaPadrao   = "1g"
	limiteProcessosPadrao = "256"
)

// OpcoesExecutorLocal configura o executor que roda o sandbox em Docker local.
// Campos vazios assumem valores padrão restritivos.
type OpcoesExecutorLocal struct {
	ImagemPorLinguagem map[Linguagem]string
	Comando            []string
	BinarioDocker      string
	Rede               string
	LimiteCPU          string
	LimiteMemoria      string
	LimiteSaidaBytes   int
}

type executorLocal struct {
	opcoes  OpcoesExecutorLocal
	comando executorDeComando
}

// NovoExecutorLocal constrói o executor que roda o sandbox em Docker local,
// usado em desenvolvimento e nos testes de integração.
func NovoExecutorLocal(opcoes OpcoesExecutorLocal) Executor {
	return novoExecutorLocal(opcoes, comandoDoSistema{})
}

func novoExecutorLocal(opcoes OpcoesExecutorLocal, comando executorDeComando) *executorLocal {
	return &executorLocal{opcoes: completarOpcoesLocais(opcoes), comando: comando}
}

func completarOpcoesLocais(opcoes OpcoesExecutorLocal) OpcoesExecutorLocal {
	opcoes.BinarioDocker = textoOuPadrao(opcoes.BinarioDocker, binarioDockerPadrao)
	opcoes.Rede = textoOuPadrao(opcoes.Rede, redePadrao)
	opcoes.LimiteCPU = textoOuPadrao(opcoes.LimiteCPU, limiteCPUPadrao)
	opcoes.LimiteMemoria = textoOuPadrao(opcoes.LimiteMemoria, limiteMemoriaPadrao)
	if opcoes.LimiteSaidaBytes <= 0 {
		opcoes.LimiteSaidaBytes = limiteSaidaPadrao
	}
	return opcoes
}

func (e *executorLocal) Executar(ctx context.Context, pedido PedidoExecucao) (ResultadoSandbox, error) {
	if err := validarPedido(pedido); err != nil {
		return ResultadoSandbox{}, err
	}

	diario := slog.With("task_run_id", pedido.TaskRunID, "executor", "local")

	imagem := e.opcoes.ImagemPorLinguagem[pedido.Linguagem]
	if imagem == "" {
		diario.Error("nenhuma imagem configurada para a linguagem", "linguagem", pedido.Linguagem)
		return resultadoDeInfraestrutura("", 0), nil
	}

	nomeDoContainer := gerarNomeDeContainer()
	inicio := time.Now()
	execucao, err := e.comando.executar(ctx, e.opcoes.BinarioDocker, e.argumentosDoRun(nomeDoContainer, imagem, pedido)...)
	duracaoMS := time.Since(inicio).Milliseconds()
	saida := truncarMantendoFinal(execucao.saida, e.opcoes.LimiteSaidaBytes)

	if ctx.Err() != nil {
		diario.Warn("execução excedeu o prazo, removendo container", "container", nomeDoContainer)
		e.removerContainer(ctx, nomeDoContainer)
		return resultadoDeInfraestrutura(saida, duracaoMS), nil
	}
	if err != nil {
		diario.Error("docker não pôde ser executado", "erro", err)
		return resultadoDeInfraestrutura(saida, duracaoMS), nil
	}

	tipo, compilou := classificarCodigoDeSaida(execucao.codigo)
	diario.Info("execução concluída", "codigo_saida", execucao.codigo, "tipo_falha", tipo, "duracao_ms", duracaoMS)

	return ResultadoSandbox{Compilou: compilou, SaidaBruta: saida, DuracaoMS: duracaoMS, TipoFalha: tipo}, nil
}

func (e *executorLocal) argumentosDoRun(nomeDoContainer, imagem string, pedido PedidoExecucao) []string {
	argumentos := []string{
		"run", "--rm",
		"--name", nomeDoContainer,
		"--user", usuarioSemPrivilegio,
		"--network", e.opcoes.Rede,
		"--cpus", e.opcoes.LimiteCPU,
		"--memory", e.opcoes.LimiteMemoria,
		"--pids-limit", limiteProcessosPadrao,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/tmp",
	}

	for _, variavel := range variaveisDoPedido(pedido) {
		argumentos = append(argumentos, "-e", variavel.nome+"="+variavel.valor)
	}

	argumentos = append(argumentos, imagem)
	return append(argumentos, e.opcoes.Comando...)
}

func (e *executorLocal) removerContainer(ctx context.Context, nomeDoContainer string) {
	ctxRemocao, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tempoLimiteRemocao)
	defer cancelar()

	if _, err := e.comando.executar(ctxRemocao, e.opcoes.BinarioDocker, "rm", "-f", nomeDoContainer); err != nil {
		slog.Error("falha ao remover container após timeout", "container", nomeDoContainer, "erro", err)
	}
}

func gerarNomeDeContainer() string {
	sufixo := make([]byte, 8)
	_, _ = rand.Read(sufixo)
	return "aprova-sandbox-" + hex.EncodeToString(sufixo)
}

func textoOuPadrao(valor, padrao string) string {
	if valor == "" {
		return padrao
	}
	return valor
}
