package acaoremota

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func capturarDiario(t *testing.T) *bytes.Buffer {
	t.Helper()

	var saida bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&saida, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(anterior) })

	return &saida
}

func entradasDoDiario(t *testing.T, saida *bytes.Buffer) []map[string]any {
	t.Helper()

	var entradas []map[string]any
	linhas := bufio.NewScanner(bytes.NewReader(saida.Bytes()))
	for linhas.Scan() {
		var entrada map[string]any
		if err := json.Unmarshal(linhas.Bytes(), &entrada); err != nil {
			t.Fatalf("linha de log não é JSON: %s", linhas.Text())
		}
		entradas = append(entradas, entrada)
	}
	return entradas
}

func unicaEntradaNoNivel(t *testing.T, saida *bytes.Buffer, nivel string) map[string]any {
	t.Helper()

	var encontradas []map[string]any
	for _, entrada := range entradasDoDiario(t, saida) {
		if entrada["level"] == nivel {
			encontradas = append(encontradas, entrada)
		}
	}
	if len(encontradas) != 1 {
		t.Fatalf("esperava uma entrada %s, encontrei %d em:\n%s", nivel, len(encontradas), saida.String())
	}
	return encontradas[0]
}

func TestServicoRegistraRecusaEmLogDeAvisoComOsCamposDaTentativa(t *testing.T) {
	saida := capturarDiario(t)
	c := novoCenario(t)

	c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-LEITOR"}))

	entrada := unicaEntradaNoNivel(t, saida, "WARN")
	esperados := map[string]any{
		"id_acao":       "acao-1",
		"tipo":          "aprovar",
		"usuario_slack": "U-LEITOR",
		"login_github":  "dave",
		"repositorio":   "Azulpasta/aPRova",
		"numero_pr":     float64(42),
		"decisao":       "recusada",
		"motivo":        "permissao_insuficiente",
	}
	for campo, valor := range esperados {
		if entrada[campo] != valor {
			t.Errorf("campo %s = %v, esperava %v", campo, entrada[campo], valor)
		}
	}
}

func TestServicoRegistraCorpoInvalidoEmLogDeAviso(t *testing.T) {
	saida := capturarDiario(t)
	c := novoCenario(t)

	c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"tipo": "mesclar"}))

	entrada := unicaEntradaNoNivel(t, saida, "WARN")
	if entrada["motivo"] != "corpo_invalido" || entrada["erro"] == nil {
		t.Errorf("a recusa por corpo inválido precisa dizer o que estava errado: %v", entrada)
	}
}

func TestServicoRegistraAceitacaoSemAviso(t *testing.T) {
	saida := capturarDiario(t)
	c := novoCenario(t)

	c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if entrada := unicaEntradaNoNivel(t, saida, "INFO"); entrada["decisao"] != "aceita" {
		t.Errorf("aceitação registrada errada: %v", entrada)
	}
	if strings.Contains(saida.String(), `"level":"WARN"`) {
		t.Error("ação aceita não é recusa e não gera aviso")
	}
}

func TestServicoRegistraFalhaDeExecucaoComoErroComACausa(t *testing.T) {
	saida := capturarDiario(t)
	c := novoCenario(t)
	c.executor.erro = errors.New("github recusou a review")

	c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if entrada := unicaEntradaNoNivel(t, saida, "ERROR"); entrada["erro"] != "github recusou a review" {
		t.Errorf("a falha precisa trazer a causa: %v", entrada)
	}
}

type registroComErro struct{}

func (registroComErro) Registrar(context.Context, Tentativa) error {
	return errors.New("armazenamento de tentativas fora do ar")
}

func TestServicoMantemARecusaQuandoORegistroFalha(t *testing.T) {
	saida := capturarDiario(t)
	c := novoCenario(t)
	c.servico.tentativas = registroComErro{}

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-LEITOR"}))

	if decisao != DecisaoRecusada {
		t.Errorf("falha ao registrar não pode mudar a decisão, obtive %q", decisao)
	}
	if !strings.Contains(saida.String(), "armazenamento de tentativas fora do ar") {
		t.Error("a falha ao registrar precisa aparecer no log")
	}
}

func TestNovoServicoExigeTodasAsDependencias(t *testing.T) {
	completas := func() OpcoesServico {
		return OpcoesServico{
			Identidades: NovoResolvedorEmMemoria(nil),
			Permissoes:  &permissoesFalsas{},
			Autoria:     &autoriaFalsa{},
			Executor:    &executorFalso{},
			Tentativas:  NovoRegistroEmMemoria(),
			Reservas:    NovaReservaEmMemoria(),
		}
	}

	casos := map[string]func(*OpcoesServico){
		"Identidades": func(o *OpcoesServico) { o.Identidades = nil },
		"Permissoes":  func(o *OpcoesServico) { o.Permissoes = nil },
		"Autoria":     func(o *OpcoesServico) { o.Autoria = nil },
		"Executor":    func(o *OpcoesServico) { o.Executor = nil },
		"Tentativas":  func(o *OpcoesServico) { o.Tentativas = nil },
		"Reservas":    func(o *OpcoesServico) { o.Reservas = nil },
	}

	for dependencia, remover := range casos {
		t.Run(dependencia, func(t *testing.T) {
			opcoes := completas()
			remover(&opcoes)

			_, err := NovoServico(opcoes)

			if !errors.Is(err, ErrDependenciaAusente) || !strings.Contains(err.Error(), dependencia) {
				t.Errorf("esperava erro nomeando %s, obtive %v", dependencia, err)
			}
		})
	}

	if _, err := NovoServico(completas()); err != nil {
		t.Errorf("com todas as dependências e sem relógio, o padrão é time.Now: %v", err)
	}
}
