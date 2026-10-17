package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func linhaDeResultado(t *testing.T, resultado ResultadoSandbox) string {
	t.Helper()

	linha, err := json.Marshal(resultado)
	if err != nil {
		t.Fatalf("serializar resultado: %v", err)
	}
	return string(linha) + "\n"
}

func executarComStdout(t *testing.T, stdout, stderr string, codigo int) ResultadoSandbox {
	t.Helper()

	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{saida: stdout, erros: stderr, codigo: codigo}, nil
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}
	return resultado
}

func TestResultadoVemDaLinhaJSONQueOExecutorDeDentroEscreve(t *testing.T) {
	casos := []ResultadoSandbox{
		{Compilou: true, TestesPassaram: 3, SaidaBruta: "ok\n", TipoFalha: FalhaNenhuma},
		{Compilou: true, TestesPassaram: 2, TestesFalharam: 1, SaidaBruta: "--- FAIL: TestFrete\n", TipoFalha: FalhaTeste},
		{Compilou: false, SaidaBruta: "syntax error\n", TipoFalha: FalhaCompilacao},
		{SaidaBruta: "aprova: repositório sem go.mod na raiz\n", TipoFalha: FalhaInfraestrutura},
	}

	for _, esperado := range casos {
		t.Run(string(esperado.TipoFalha), func(t *testing.T) {
			obtido := executarComStdout(t, linhaDeResultado(t, esperado), "log do executor em stderr\n", 0)

			obtido.DuracaoMS = 0
			if obtido != esperado {
				t.Errorf("obtive %+v, esperava %+v", obtido, esperado)
			}
		})
	}
}

func TestResultadoNaoDependeDoCodigoDeSaidaDoContainer(t *testing.T) {
	esperado := ResultadoSandbox{Compilou: true, TestesFalharam: 1, TipoFalha: FalhaTeste}

	obtido := executarComStdout(t, linhaDeResultado(t, esperado), "", 1)

	if obtido.TipoFalha != FalhaTeste {
		t.Errorf("a linha JSON manda, não o código de saída; obtive %+v", obtido)
	}
}

func TestStdoutSemResultadoValidoEhInfraestrutura(t *testing.T) {
	casos := []struct {
		descricao string
		stdout    string
		codigo    int
	}{
		{"docker não subiu o container, imagem ausente", "", 125},
		{"executor de dentro morto antes de publicar", "", 137},
		{"linha que não é JSON", "não sou um resultado\n", 0},
		{"JSON com tipo de falha desconhecido", `{"compilou":true,"tipo_falha":"sucesso_total"}` + "\n", 0},
		{"JSON sem tipo de falha", `{"compilou":true,"testes_passaram":99}` + "\n", 0},
	}

	for _, caso := range casos {
		t.Run(caso.descricao, func(t *testing.T) {
			resultado := executarComStdout(t, caso.stdout, "Unable to find image 'imagem-de-teste'\n", caso.codigo)

			if resultado.TipoFalha != FalhaInfraestrutura || resultado.Compilou || resultado.TestesPassaram != 0 {
				t.Errorf("sem resultado confiável é infraestrutura, obtive %+v", resultado)
			}
			if !strings.Contains(resultado.SaidaBruta, "Unable to find image") {
				t.Errorf("o stderr do docker explica o que houve e precisa estar na saída: %q", resultado.SaidaBruta)
			}
		})
	}
}

func TestDockerAusenteEFalhaDeInfraestrutura(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, errors.New(`exec: "docker": executable file not found in $PATH`)
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())

	if err != nil {
		t.Fatalf("falha de infraestrutura é resultado, não erro de chamada: %v", err)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("sem docker a culpa é nossa; esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
}

func TestContainerSobeIsoladoELimitado(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}

	if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido()); err != nil {
		t.Fatalf("executar: %v", err)
	}
	argumentos := comando.argumentosDoRun(t)

	for _, obrigatorio := range []string{"--rm", "--read-only"} {
		if !slices.Contains(argumentos, obrigatorio) {
			t.Errorf("faltou %s nos argumentos: %v", obrigatorio, argumentos)
		}
	}

	exigencias := map[string]string{
		"--network":      redePadrao,
		"--cpus":         limiteCPUPadrao,
		"--memory":       limiteMemoriaPadrao,
		"--pids-limit":   limiteProcessosPadrao,
		"--cap-drop":     "ALL",
		"--security-opt": "no-new-privileges",
		"--tmpfs":        "/tmp:rw,exec,nosuid,nodev,size=1g",
	}
	for flag, esperado := range exigencias {
		if obtido := valorDaFlag(argumentos, flag); obtido != esperado {
			t.Errorf("%s deveria ser %q, obtive %q", flag, esperado, obtido)
		}
	}

	if capacidades := valoresDaFlag(argumentos, "--cap-add"); !slices.Equal(capacidades, []string{"SETUID", "SETGID"}) {
		t.Errorf("o executor de dentro só pode trocar de usuário: --cap-add %v", capacidades)
	}
	if slices.Contains(argumentos, "--user") {
		t.Error("o executor de dentro roda como root sem capacidades; o código do PR roda como 10001 por dentro")
	}
}

func valoresDaFlag(argumentos []string, flag string) []string {
	var valores []string
	for indice, argumento := range argumentos {
		if argumento == flag && indice+1 < len(argumentos) {
			valores = append(valores, argumentos[indice+1])
		}
	}
	return valores
}

func TestContainerRecebePrazoComMargemESaidaNoStdout(t *testing.T) {
	casos := []struct {
		descricao string
		prazo     time.Duration
		aceitos   []string
	}{
		{"prazo de 90s deixa 15s para o container subir", 90 * time.Second, []string{"75", "74"}},
		{"sem prazo no contexto usa o padrão", 0, []string{"600"}},
	}

	for _, caso := range casos {
		t.Run(caso.descricao, func(t *testing.T) {
			comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
				return resultadoComando{}, nil
			}}
			ctx := t.Context()
			if caso.prazo > 0 {
				var cancelar context.CancelFunc
				ctx, cancelar = context.WithTimeout(ctx, caso.prazo)
				defer cancelar()
			}

			if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(ctx, pedidoValido()); err != nil {
				t.Fatalf("executar: %v", err)
			}
			argumentos := comando.argumentosDoRun(t)

			aceito := false
			for _, segundos := range caso.aceitos {
				aceito = aceito || contemVariavel(argumentos, "APROVA_PRAZO_SEGUNDOS="+segundos)
			}
			if !aceito {
				t.Errorf("esperava APROVA_PRAZO_SEGUNDOS em %v: %v", caso.aceitos, argumentos)
			}
			if !contemVariavel(argumentos, "APROVA_SAIDA=stdout") {
				t.Errorf("o executor local lê o resultado do stdout: %v", argumentos)
			}
		})
	}
}

func TestContainerMontaOsVolumesConfigurados(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}
	opcoes := opcoesLocaisDeTeste()
	opcoes.Montagens = []string{"/host/repos:/repositorios:ro"}

	if _, err := novoExecutorLocal(opcoes, comando).Executar(t.Context(), pedidoValido()); err != nil {
		t.Fatalf("executar: %v", err)
	}

	if montagens := valoresDaFlag(comando.argumentosDoRun(t), "-v"); !slices.Equal(montagens, opcoes.Montagens) {
		t.Errorf("montagens = %v, esperava %v", montagens, opcoes.Montagens)
	}
}

func TestContainerRecebeOPedidoPorVariaveisDeAmbiente(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}
	pedido := pedidoValido()

	if _, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido); err != nil {
		t.Fatalf("executar: %v", err)
	}
	argumentos := comando.argumentosDoRun(t)

	for _, esperado := range []string{
		"APROVA_REPOSITORIO=" + pedido.URLRepositorio,
		"APROVA_SHA=" + pedido.SHA,
		"APROVA_LINGUAGEM=" + string(pedido.Linguagem),
		"APROVA_TASK_RUN_ID=" + pedido.TaskRunID,
	} {
		if !contemVariavel(argumentos, esperado) {
			t.Errorf("faltou -e %s nos argumentos: %v", esperado, argumentos)
		}
	}

	if imagem := argumentos[len(argumentos)-1]; imagem != "imagem-de-teste" {
		t.Errorf("a imagem da linguagem deveria ser o último argumento, obtive %q", imagem)
	}
}

func TestDuracaoDaExecucaoEMedida(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		time.Sleep(20 * time.Millisecond)
		return resultadoComando{}, nil
	}}

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedidoValido())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}
	if resultado.DuracaoMS < 20 {
		t.Errorf("a execução levou ao menos 20ms, registrou %dms", resultado.DuracaoMS)
	}
}

func TestLinguagemSemImagemConfiguradaEFalhaDeInfraestruturaSemChamarDocker(t *testing.T) {
	comando := comandoQueNaoDeveSerChamado(t)
	pedido := pedidoValido()
	pedido.Linguagem = LinguagemPython

	resultado, err := novoExecutorLocal(opcoesLocaisDeTeste(), comando).Executar(t.Context(), pedido)

	if err != nil {
		t.Fatalf("imagem não configurada é problema nosso, resultado e não erro: %v", err)
	}
	if resultado.TipoFalha != FalhaInfraestrutura {
		t.Errorf("esperava %q, obtive %q", FalhaInfraestrutura, resultado.TipoFalha)
	}
}

func contemVariavel(argumentos []string, atribuicao string) bool {
	for indice, argumento := range argumentos {
		if argumento == "-e" && indice+1 < len(argumentos) && argumentos[indice+1] == atribuicao {
			return true
		}
	}
	return false
}

func TestContainerSemCapacidadeDeTrocarDeUsuarioDerrubaTudo(t *testing.T) {
	comando := &comandoFalso{responder: func(_ context.Context, _ []string) (resultadoComando, error) {
		return resultadoComando{}, nil
	}}
	opcoes := opcoesLocaisDeTeste()
	opcoes.SemCapacidadeDeTrocarDeUsuario = true

	if _, err := novoExecutorLocal(opcoes, comando).Executar(t.Context(), pedidoValido()); err != nil {
		t.Fatalf("executar: %v", err)
	}
	argumentos := comando.argumentosDoRun(t)

	if capacidades := valoresDaFlag(argumentos, "--cap-add"); len(capacidades) != 0 {
		t.Errorf("simulando um ambiente que nega a troca, nenhuma capacidade volta: --cap-add %v", capacidades)
	}
	if valorDaFlag(argumentos, "--cap-drop") != "ALL" {
		t.Errorf("--cap-drop ALL continua obrigatório: %v", argumentos)
	}
}
