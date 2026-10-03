package sandbox

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type clienteDeJobsFalso struct {
	mutex              sync.Mutex
	statusEmSequencia  []string
	consultas          int
	erroAoIniciar      error
	variaveisRecebidas map[string]string
	execucoesParadas   []string
}

func (c *clienteDeJobsFalso) Iniciar(_ context.Context, variaveis map[string]string) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.erroAoIniciar != nil {
		return "", c.erroAoIniciar
	}
	c.variaveisRecebidas = variaveis
	return "execucao-1", nil
}

func (c *clienteDeJobsFalso) Status(_ context.Context, _ string) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	indice := min(c.consultas, len(c.statusEmSequencia)-1)
	c.consultas++
	return c.statusEmSequencia[indice], nil
}

func (c *clienteDeJobsFalso) Parar(_ context.Context, idExecucao string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.execucoesParadas = append(c.execucoesParadas, idExecucao)
	return nil
}

func (c *clienteDeJobsFalso) foiParada(idExecucao string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return slices.Contains(c.execucoesParadas, idExecucao)
}

func opcoesAzureDeTeste() OpcoesExecutorAzure {
	return OpcoesExecutorAzure{IntervaloConsulta: time.Millisecond, PrazoInicio: time.Second}
}

func executarNaAzure(t *testing.T, cliente *clienteDeJobsFalso, coletor ColetorResultado, opcoes OpcoesExecutorAzure, prazo time.Duration) ResultadoSandbox {
	t.Helper()

	ctx, cancelar := context.WithTimeout(t.Context(), prazo)
	defer cancelar()

	resultado, err := novoExecutorAzure(opcoes, cliente, coletor).Executar(ctx, pedidoValido())
	if err != nil {
		t.Fatalf("falha na Azure é resultado classificado, não erro de chamada: %v", err)
	}
	return resultado
}

func TestDisparoDoJobEnviaAsVariaveisDoPedido(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Succeeded"}}
	coletor := NovoColetorEmMemoria()
	_ = coletor.Gravar(t.Context(), pedidoValido().TaskRunID, resultadoDeExemplo())

	executarNaAzure(t, cliente, coletor, opcoesAzureDeTeste(), time.Second)

	pedido := pedidoValido()
	esperadas := map[string]string{
		"APROVA_REPOSITORIO": pedido.URLRepositorio,
		"APROVA_SHA":         pedido.SHA,
		"APROVA_LINGUAGEM":   string(pedido.Linguagem),
		"APROVA_TASK_RUN_ID": pedido.TaskRunID,
	}
	for nome, valor := range esperadas {
		if cliente.variaveisRecebidas[nome] != valor {
			t.Errorf("%s deveria ser %q, obtive %q", nome, valor, cliente.variaveisRecebidas[nome])
		}
	}
}

func TestExecucaoConcluidaEntregaOResultadoDoColetor(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Processing", "Running", "Running", "Succeeded"}}
	coletor := NovoColetorEmMemoria()
	gravado := ResultadoSandbox{Compilou: true, TestesFalharam: 2, TipoFalha: FalhaTeste, SaidaBruta: "2 falhas"}
	_ = coletor.Gravar(t.Context(), pedidoValido().TaskRunID, gravado)

	resultado := executarNaAzure(t, cliente, coletor, opcoesAzureDeTeste(), time.Second)

	if resultado != gravado {
		t.Errorf("o resultado vem do coletor, não dos logs da Azure; esperava %+v, obtive %+v", gravado, resultado)
	}
}

func TestExecucaoQueTerminaMalNaAzureEFalhaDeInfraestrutura(t *testing.T) {
	for _, status := range []string{"Failed", "Stopped", "Degraded"} {
		t.Run(status, func(t *testing.T) {
			cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Running", status}}

			resultado := executarNaAzure(t, cliente, NovoColetorEmMemoria(), opcoesAzureDeTeste(), time.Second)

			if resultado.TipoFalha != FalhaInfraestrutura {
				t.Errorf("status %s é culpa nossa; esperava %q, obtive %q", status, FalhaInfraestrutura, resultado.TipoFalha)
			}
		})
	}
}

func TestExecucaoQueNuncaTerminaRespeitaOContextoEEParada(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Running"}}

	inicio := time.Now()
	resultado := executarNaAzure(t, cliente, NovoColetorEmMemoria(), opcoesAzureDeTeste(), 50*time.Millisecond)

	if decorrido := time.Since(inicio); decorrido > time.Second {
		t.Errorf("o polling deveria parar junto com o contexto, levou %v", decorrido)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
	if !cliente.foiParada("execucao-1") {
		t.Error("execução abandonada continua sendo cobrada; deveria ter sido parada")
	}
}

func TestExecucaoQueNaoIniciaDentroDoPrazoEFalhaDeInfraestrutura(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Processing"}}
	opcoes := opcoesAzureDeTeste()
	opcoes.PrazoInicio = 20 * time.Millisecond

	resultado := executarNaAzure(t, cliente, NovoColetorEmMemoria(), opcoes, 5*time.Second)

	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
	if !cliente.foiParada("execucao-1") {
		t.Error("a execução que não iniciou deveria ser cancelada")
	}
}

func TestErroAoDispararOJobEFalhaDeInfraestrutura(t *testing.T) {
	cliente := &clienteDeJobsFalso{erroAoIniciar: errors.New("AuthorizationFailed")}

	resultado := executarNaAzure(t, cliente, NovoColetorEmMemoria(), opcoesAzureDeTeste(), time.Second)

	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
}

func TestPedidoInvalidoNaoDisparaJobNaAzure(t *testing.T) {
	cliente := &clienteDeJobsFalso{statusEmSequencia: []string{"Succeeded"}}
	pedido := pedidoValido()
	pedido.Linguagem = "cobol"

	_, err := novoExecutorAzure(opcoesAzureDeTeste(), cliente, NovoColetorEmMemoria()).Executar(t.Context(), pedido)

	if !errors.Is(err, ErrPedidoInvalido) {
		t.Errorf("esperava ErrPedidoInvalido, obtive %v", err)
	}
	if cliente.variaveisRecebidas != nil {
		t.Error("nenhum job pode ser disparado para pedido inválido")
	}
}
