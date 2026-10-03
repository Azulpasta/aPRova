package sandbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox/jobsazure"
)

const (
	intervaloConsultaPadrao = 10 * time.Second
	prazoInicioPadrao       = 5 * time.Minute
	tempoLimiteParada       = 30 * time.Second
)

type situacaoExecucao int

const (
	situacaoAguardandoInicio situacaoExecucao = iota
	situacaoRodando
	situacaoConcluida
	situacaoFalhou
)

// OpcoesExecutorAzure configura o executor que dispara o job do sandbox no
// Azure Container Apps. Intervalo e prazo vazios assumem valores padrão.
type OpcoesExecutorAzure struct {
	SubscriptionID    string
	TenantID          string
	ClientID          string
	ClientSecret      string
	ResourceGroup     string
	NomeDoJob         string
	IntervaloConsulta time.Duration
	PrazoInicio       time.Duration
}

type clienteDeJobs interface {
	Iniciar(ctx context.Context, variaveis map[string]string) (string, error)
	Status(ctx context.Context, idExecucao string) (string, error)
	Parar(ctx context.Context, idExecucao string) error
}

type executorAzure struct {
	opcoes  OpcoesExecutorAzure
	cliente clienteDeJobs
	coletor ColetorResultado
}

func novoExecutorAzure(opcoes OpcoesExecutorAzure, cliente clienteDeJobs, coletor ColetorResultado) *executorAzure {
	if opcoes.IntervaloConsulta <= 0 {
		opcoes.IntervaloConsulta = intervaloConsultaPadrao
	}
	if opcoes.PrazoInicio <= 0 {
		opcoes.PrazoInicio = prazoInicioPadrao
	}
	return &executorAzure{opcoes: opcoes, cliente: cliente, coletor: coletor}
}

func (e *executorAzure) Executar(ctx context.Context, pedido PedidoExecucao) (ResultadoSandbox, error) {
	if err := validarPedido(pedido); err != nil {
		return ResultadoSandbox{}, err
	}

	diario := slog.With("task_run_id", pedido.TaskRunID, "executor", "azure")
	inicio := time.Now()

	idExecucao, err := e.cliente.Iniciar(ctx, mapaDeVariaveis(pedido))
	if err != nil {
		diario.Error("falha ao disparar o job na azure", "erro", err)
		return resultadoDeInfraestrutura("", time.Since(inicio).Milliseconds()), nil
	}

	diario = diario.With("execucao_azure", idExecucao)
	if e.acompanhar(ctx, idExecucao, diario) != situacaoConcluida {
		return resultadoDeInfraestrutura("", time.Since(inicio).Milliseconds()), nil
	}

	resultado, err := e.coletor.Aguardar(ctx, pedido.TaskRunID)
	if err != nil {
		diario.Error("execução concluída sem resultado no coletor", "erro", err)
		return resultadoDeInfraestrutura("", time.Since(inicio).Milliseconds()), nil
	}

	return resultado, nil
}

func (e *executorAzure) acompanhar(ctx context.Context, idExecucao string, diario *slog.Logger) situacaoExecucao {
	limiteParaIniciar := time.Now().Add(e.opcoes.PrazoInicio)
	iniciou := false

	for {
		situacao := e.consultarSituacao(ctx, idExecucao, diario)
		if situacao == situacaoConcluida || situacao == situacaoFalhou {
			return situacao
		}
		iniciou = iniciou || situacao == situacaoRodando

		if !iniciou && time.Now().After(limiteParaIniciar) {
			diario.Error("execução não iniciou dentro do prazo", "prazo", e.opcoes.PrazoInicio)
			e.parar(ctx, idExecucao, diario)
			return situacaoFalhou
		}

		select {
		case <-ctx.Done():
			diario.Warn("prazo esgotado acompanhando a execução, parando o job")
			e.parar(ctx, idExecucao, diario)
			return situacaoFalhou
		case <-time.After(e.opcoes.IntervaloConsulta):
		}
	}
}

func (e *executorAzure) consultarSituacao(ctx context.Context, idExecucao string, diario *slog.Logger) situacaoExecucao {
	status, err := e.cliente.Status(ctx, idExecucao)
	if err != nil {
		diario.Warn("falha ao consultar status da execução, tentando de novo", "erro", err)
		return situacaoAguardandoInicio
	}

	situacao := interpretarStatusAzure(status)
	if situacao == situacaoConcluida || situacao == situacaoFalhou {
		diario.Info("execução terminou na azure", "status", status)
	}
	return situacao
}

func (e *executorAzure) parar(ctx context.Context, idExecucao string, diario *slog.Logger) {
	ctxParada, cancelar := context.WithTimeout(context.WithoutCancel(ctx), tempoLimiteParada)
	defer cancelar()

	if err := e.cliente.Parar(ctxParada, idExecucao); err != nil {
		diario.Error("falha ao parar a execução na azure", "erro", err)
	}
}

func interpretarStatusAzure(status string) situacaoExecucao {
	switch {
	case status == jobsazure.StatusConcluido:
		return situacaoConcluida
	case jobsazure.StatusDeFalha(status):
		return situacaoFalhou
	case status == jobsazure.StatusRodando:
		return situacaoRodando
	default:
		return situacaoAguardandoInicio
	}
}

func mapaDeVariaveis(pedido PedidoExecucao) map[string]string {
	mapa := map[string]string{}
	for _, variavel := range variaveisDoPedido(pedido) {
		mapa[variavel.nome] = variavel.valor
	}
	return mapa
}
