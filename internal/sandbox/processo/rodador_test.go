package processo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func rodar(t *testing.T, ctx context.Context, comando Comando) Execucao {
	t.Helper()
	return NovoRodador(Opcoes{EsperaAposEncerrar: 500 * time.Millisecond}).Rodar(ctx, comando)
}

func TestRodarCapturaStdoutEStderrNaOrdem(t *testing.T) {
	execucao := rodar(t, t.Context(), Comando{
		Binario:    "/bin/sh",
		Argumentos: []string{"-c", "echo um; echo dois >&2; echo tres"},
	})

	if execucao.Erro != nil || execucao.CodigoSaida != 0 {
		t.Fatalf("comando simples falhou: %+v", execucao)
	}
	if string(execucao.Saida) != "um\ndois\ntres\n" {
		t.Errorf("saída = %q", execucao.Saida)
	}
}

func TestRodarDevolveOCodigoDeSaida(t *testing.T) {
	execucao := rodar(t, t.Context(), Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "exit 3"}})

	if execucao.Erro != nil || execucao.CodigoSaida != 3 || execucao.PrazoEsgotado {
		t.Errorf("esperava código 3 sem erro, obtive %+v", execucao)
	}
}

func TestRodarSinalizaBinarioInexistente(t *testing.T) {
	execucao := rodar(t, t.Context(), Comando{Binario: "/caminho/que/nao/existe"})

	if execucao.Erro == nil {
		t.Error("binário inexistente precisa virar erro de início")
	}
}

func TestRodarNaoHerdaOAmbienteDoExecutor(t *testing.T) {
	t.Setenv("SEGREDO_DO_EXECUTOR", "nao-pode-vazar")

	execucao := rodar(t, t.Context(), Comando{Binario: "/usr/bin/env", Ambiente: []string{"SOMENTE=esta"}})

	if string(execucao.Saida) != "SOMENTE=esta\n" {
		t.Errorf("o filho só pode ver o ambiente informado, viu %q", execucao.Saida)
	}
}

func TestRodarSemAmbienteNaoHerdaNada(t *testing.T) {
	t.Setenv("SEGREDO_DO_EXECUTOR", "nao-pode-vazar")

	execucao := rodar(t, t.Context(), Comando{Binario: "/usr/bin/env"})

	if len(execucao.Saida) != 0 {
		t.Errorf("ambiente vazio não pode virar herança, o filho viu %q", execucao.Saida)
	}
}

func TestRodarNoDiretorioInformado(t *testing.T) {
	diretorio := t.TempDir()

	execucao := rodar(t, t.Context(), Comando{Binario: "/bin/pwd", Argumentos: []string{"-P"}, Diretorio: diretorio})

	if !strings.HasSuffix(strings.TrimSpace(string(execucao.Saida)), strings.TrimPrefix(diretorio, "/private")) {
		t.Errorf("pwd = %q, esperava %q", execucao.Saida, diretorio)
	}
}

func TestRodarEncerraOProcessoQuandoOPrazoAcaba(t *testing.T) {
	ctx, cancelar := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancelar()
	inicio := time.Now()

	execucao := rodar(t, ctx, Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "echo comecou; sleep 30"}})

	if !execucao.PrazoEsgotado {
		t.Errorf("esperava prazo esgotado, obtive %+v", execucao)
	}
	if decorrido := time.Since(inicio); decorrido > 5*time.Second {
		t.Errorf("o processo deveria ter sido encerrado no prazo, levou %v", decorrido)
	}
	if !strings.Contains(string(execucao.Saida), "comecou") {
		t.Errorf("a saída produzida antes do prazo precisa ser preservada: %q", execucao.Saida)
	}
}

func TestRodarNaoFicaPresoEmProcessoQueSegueComOPipeAberto(t *testing.T) {
	inicio := time.Now()

	execucao := rodar(t, t.Context(), Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "sleep 30 & echo pronto"}})

	if decorrido := time.Since(inicio); decorrido > 5*time.Second {
		t.Fatalf("um filho em segundo plano segurou o pipe por %v", decorrido)
	}
	if execucao.Erro != nil || execucao.CodigoSaida != 0 || !strings.Contains(string(execucao.Saida), "pronto") {
		t.Errorf("o comando principal terminou bem, obtive %+v", execucao)
	}
}

func credencialSemPrivilegio() *Credencial {
	return &Credencial{UID: 10001, GID: 10001}
}

func TestRodarFalhaFechadoQuandoATrocaDeUsuarioENegada(t *testing.T) {
	tentativas := 0
	rodador := NovoRodador(Opcoes{Credencial: credencialSemPrivilegio()})
	rodador.iniciar = func(processo *exec.Cmd) error {
		tentativas++
		if processo.SysProcAttr == nil || processo.SysProcAttr.Credential == nil {
			t.Error("toda tentativa precisa carregar a credencial sem privilégio")
		}
		return &os.PathError{Op: "fork/exec", Path: processo.Path, Err: syscall.EPERM}
	}

	execucao := rodador.Rodar(t.Context(), Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "echo rodou"}})

	if !errors.Is(execucao.Erro, ErrTrocaDeUsuarioNegada) {
		t.Errorf("esperava ErrTrocaDeUsuarioNegada, obtive %v", execucao.Erro)
	}
	if !errors.Is(execucao.Erro, syscall.EPERM) {
		t.Errorf("a causa EPERM precisa continuar na cadeia de erros para o diagnóstico, obtive %v", execucao.Erro)
	}
	if tentativas != 1 {
		t.Errorf("a troca negada não pode ser repetida nem contornada, houve %d tentativas", tentativas)
	}
	if len(execucao.Saida) != 0 || execucao.CodigoSaida != 0 {
		t.Errorf("nada pode ter rodado, obtive %+v", execucao)
	}
}

func TestRodarSemCredencialNaoConfundeEPERMComTrocaNegada(t *testing.T) {
	rodador := NovoRodador(Opcoes{})
	rodador.iniciar = func(processo *exec.Cmd) error {
		return &os.PathError{Op: "fork/exec", Path: processo.Path, Err: syscall.EPERM}
	}

	execucao := rodador.Rodar(t.Context(), Comando{Binario: "/bin/sh"})

	if execucao.Erro == nil || errors.Is(execucao.Erro, ErrTrocaDeUsuarioNegada) {
		t.Errorf("sem credencial não houve troca de usuário a negar, obtive %v", execucao.Erro)
	}
}

func TestRodarComPrazoJaEsgotadoInformaPrazoESemRodar(t *testing.T) {
	ctx, cancelar := context.WithCancel(t.Context())
	cancelar()

	execucao := rodar(t, ctx, Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "echo rodou"}})

	if !execucao.PrazoEsgotado || execucao.Erro != nil {
		t.Errorf("contexto vencido antes de começar é prazo esgotado, obtive %+v", execucao)
	}
	if len(execucao.Saida) != 0 {
		t.Errorf("nada pode ter rodado: %q", execucao.Saida)
	}
}

func TestRodadorComValorZeroContinuaUtilizavel(t *testing.T) {
	var rodador RodadorDoSistema

	execucao := rodador.Rodar(t.Context(), Comando{Binario: "/bin/sh", Argumentos: []string{"-c", "echo ok"}})

	if execucao.Erro != nil || string(execucao.Saida) != "ok\n" {
		t.Errorf("o valor zero roda com o usuário atual, obtive %+v", execucao)
	}
}
