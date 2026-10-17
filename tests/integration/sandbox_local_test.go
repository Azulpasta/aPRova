package integracao

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

const (
	imagemDeTeste         = "alpine:3.20"
	prefixoDosConteineres = "aprova-sandbox-"
	tempoLimiteDoDocker   = 10 * time.Second
	tempoLimiteDeDownload = 3 * time.Minute
)

func exigirDocker(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker não está instalado")
	}

	ctx, cancelar := context.WithTimeout(t.Context(), tempoLimiteDoDocker)
	defer cancelar()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skip("o daemon do docker não está respondendo")
	}

	ctxDownload, cancelarDownload := context.WithTimeout(t.Context(), tempoLimiteDeDownload)
	defer cancelarDownload()
	if saida, err := exec.CommandContext(ctxDownload, "docker", "pull", "-q", imagemDeTeste).CombinedOutput(); err != nil {
		t.Fatalf("não foi possível baixar %s: %v\n%s", imagemDeTeste, err, saida)
	}

	t.Cleanup(func() { exigirNenhumConteinerSobrando(t) })
}

func conteineresDoSandbox(t *testing.T) []string {
	t.Helper()

	ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteDoDocker)
	defer cancelar()

	saida, err := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "name="+prefixoDosConteineres, "--format", "{{.Names}}").Output()
	if err != nil {
		t.Fatalf("listar containers: %v", err)
	}
	return strings.Fields(string(saida))
}

func exigirNenhumConteinerSobrando(t *testing.T) {
	t.Helper()

	if sobrando := conteineresDoSandbox(t); len(sobrando) > 0 {
		t.Errorf("containers do sandbox continuam existindo após o teste: %v", sobrando)
	}
}

func executorComComando(comando ...string) sandbox.Executor {
	return sandbox.NovoExecutorLocal(sandbox.OpcoesExecutorLocal{
		ImagemPorLinguagem: map[sandbox.Linguagem]string{sandbox.LinguagemGo: imagemDeTeste},
		Comando:            comando,
	})
}

func pedidoDeTeste() sandbox.PedidoExecucao {
	return sandbox.PedidoExecucao{
		URLRepositorio: "https://github.com/Azulpasta/aPRova.git",
		SHA:            "3f2c1e0b9a8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b",
		Linguagem:      sandbox.LinguagemGo,
		TaskRunID:      "task-run-integracao",
	}
}

func TestContainerQuePublicaALinhaDeResultadoEntregaEsseResultado(t *testing.T) {
	exigirDocker(t)

	ctx, cancelar := context.WithTimeout(t.Context(), time.Minute)
	defer cancelar()

	resultado, err := executorComComando("sh", "-c",
		`echo "log qualquer no stderr" >&2; printf '{"compilou":true,"testes_passaram":4,"saida_bruta":"sha %s","tipo_falha":"nenhuma"}\n' "$APROVA_SHA"`,
	).Executar(ctx, pedidoDeTeste())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}

	if resultado.TipoFalha != sandbox.FalhaNenhuma || resultado.TestesPassaram != 4 {
		t.Errorf("o resultado vem da linha JSON do stdout, obtive %+v", resultado)
	}
	if resultado.SaidaBruta != "sha "+pedidoDeTeste().SHA {
		t.Errorf("o container recebeu o pedido por variável de ambiente? saída %q", resultado.SaidaBruta)
	}
}

func TestContainerSemResultadoPublicadoEhInfraestruturaEPreservaASaida(t *testing.T) {
	exigirDocker(t)

	ctx, cancelar := context.WithTimeout(t.Context(), time.Minute)
	defer cancelar()

	resultado, err := executorComComando("sh", "-c", `echo "morri antes de publicar"; exit 0`).Executar(ctx, pedidoDeTeste())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "morri antes de publicar") {
		t.Errorf("sem linha de resultado é infraestrutura com a saída preservada, obtive %+v", resultado)
	}
}

func TestContainerSoTemCapacidadeDeTrocarDeUsuarioENaoTemRede(t *testing.T) {
	exigirDocker(t)

	ctx, cancelar := context.WithTimeout(t.Context(), time.Minute)
	defer cancelar()

	resultado, err := executorComComando("sh", "-c",
		`grep CapEff /proc/self/status; wget -q -T 2 -O /dev/null http://example.com && echo "rede=aberta" || echo "rede=fechada"`,
	).Executar(ctx, pedidoDeTeste())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}

	const apenasSetgidESetuid = "CapEff:\t00000000000000c0"
	if !strings.Contains(resultado.SaidaBruta, apenasSetgidESetuid) {
		t.Errorf("o executor de dentro só pode ter SETUID e SETGID: %s", resultado.SaidaBruta)
	}
	if !strings.Contains(resultado.SaidaBruta, "rede=fechada") {
		t.Errorf("o container alcançou a rede: %s", resultado.SaidaBruta)
	}
}

func TestContainerQueExcedeOPrazoEMortoRemovidoEFalhaDeInfraestrutura(t *testing.T) {
	exigirDocker(t)

	ctx, cancelar := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelar()

	inicio := time.Now()
	resultado, err := executorComComando("sleep", "60").Executar(ctx, pedidoDeTeste())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("timeout é culpa nossa; esperava %q, obtive %q", sandbox.FalhaInfraestrutura, resultado.TipoFalha)
	}
	if decorrido := time.Since(inicio); decorrido > 30*time.Second {
		t.Errorf("o executor deveria desistir junto com o prazo, levou %v", decorrido)
	}
	if sobrando := conteineresDoSandbox(t); len(sobrando) > 0 {
		t.Errorf("o container que estourou o prazo foi abandonado em vez de removido: %v", sobrando)
	}
}

func TestImagemInexistenteEFalhaDeInfraestruturaENaoDoPR(t *testing.T) {
	exigirDocker(t)

	ctx, cancelar := context.WithTimeout(t.Context(), time.Minute)
	defer cancelar()

	executor := sandbox.NovoExecutorLocal(sandbox.OpcoesExecutorLocal{
		ImagemPorLinguagem: map[sandbox.Linguagem]string{sandbox.LinguagemGo: "aprova/imagem-que-nao-existe:0"},
	})

	resultado, err := executor.Executar(ctx, pedidoDeTeste())
	if err != nil {
		t.Fatalf("executar: %v", err)
	}

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("imagem ausente é culpa nossa; esperava %q, obtive %q", sandbox.FalhaInfraestrutura, resultado.TipoFalha)
	}
}
