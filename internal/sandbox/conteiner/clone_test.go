package conteiner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

func exigirGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git não está instalado")
	}
}

func git(t *testing.T, diretorio string, argumentos ...string) string {
	t.Helper()

	comando := exec.CommandContext(t.Context(), "git", append([]string{"-C", diretorio}, argumentos...)...)
	comando.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	saida, err := comando.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", argumentos, err, saida)
	}
	return strings.TrimSpace(string(saida))
}

func repositorioComDoisCommits(t *testing.T) (endereco, shaAntigo string) {
	t.Helper()

	origem := t.TempDir()
	git(t, origem, "init", "-q")
	if err := os.WriteFile(filepath.Join(origem, "frete.go"), []byte("versao antiga\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, origem, "add", ".")
	git(t, origem, "commit", "-q", "-m", "antigo")
	shaAntigo = git(t, origem, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(origem, "frete.go"), []byte("versao nova\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, origem, "commit", "-q", "-am", "novo")

	return "file://" + origem, shaAntigo
}

func clonador() Clonador {
	return Clonador{Rodador: processo.NovoRodador(processo.Opcoes{})}
}

func TestClonarPosicionaNoSHAExatoMesmoQueNaoSejaAPonta(t *testing.T) {
	exigirGit(t)
	endereco, shaAntigo := repositorioComDoisCommits(t)
	destino := filepath.Join(t.TempDir(), "repositorio")

	if err := clonador().Clonar(t.Context(), endereco, shaAntigo, destino); err != nil {
		t.Fatalf("clone não deveria falhar: %v", err)
	}

	conteudo, err := os.ReadFile(filepath.Join(destino, "frete.go"))
	if err != nil || string(conteudo) != "versao antiga\n" {
		t.Errorf("o checkout precisa estar no SHA pedido, frete.go = %q (%v)", conteudo, err)
	}
}

func TestClonarFalhaComSHAInexistente(t *testing.T) {
	exigirGit(t)
	endereco, _ := repositorioComDoisCommits(t)

	err := clonador().Clonar(t.Context(), endereco, strings.Repeat("a", 40), filepath.Join(t.TempDir(), "repositorio"))

	if !errors.Is(err, ErrClone) || !strings.Contains(err.Error(), "git fetch") {
		t.Errorf("esperava ErrClone citando o fetch, obtive %v", err)
	}
}

func TestClonarFalhaComRepositorioInexistente(t *testing.T) {
	exigirGit(t)

	err := clonador().Clonar(t.Context(), "file:///nao/existe.git", strings.Repeat("a", 40), filepath.Join(t.TempDir(), "repositorio"))

	if !errors.Is(err, ErrClone) {
		t.Errorf("esperava ErrClone, obtive %v", err)
	}
}

type rodadorDeGitFalso struct {
	headDevolvido string
	comandos      []processo.Comando
}

func (r *rodadorDeGitFalso) Rodar(_ context.Context, comando processo.Comando) processo.Execucao {
	r.comandos = append(r.comandos, comando)
	if slices.Contains(comando.Argumentos, "rev-parse") {
		return processo.Execucao{Saida: []byte(r.headDevolvido + "\n")}
	}
	return processo.Execucao{}
}

func TestClonarRecusaHEADDiferenteDoSHAPedido(t *testing.T) {
	rodador := &rodadorDeGitFalso{headDevolvido: strings.Repeat("b", 40)}

	err := Clonador{Rodador: rodador}.Clonar(t.Context(), "https://github.com/x/y.git", shaDeTeste, "/tmp/repositorio")

	if !errors.Is(err, ErrClone) || !strings.Contains(err.Error(), "diverge") {
		t.Errorf("HEAD divergente precisa falhar, obtive %v", err)
	}
}

func TestClonarRodaOGitSemCredencialNemPerguntaInterativa(t *testing.T) {
	rodador := &rodadorDeGitFalso{headDevolvido: shaDeTeste}

	if err := (Clonador{Rodador: rodador}).Clonar(t.Context(), "https://github.com/x/y.git", shaDeTeste, "/tmp/repositorio"); err != nil {
		t.Fatalf("clone não deveria falhar: %v", err)
	}

	if len(rodador.comandos) == 0 {
		t.Fatal("nenhum comando git rodou")
	}
	for _, comando := range rodador.comandos {
		if comando.Binario != "git" {
			t.Errorf("comando inesperado: %+v", comando)
		}
		for _, exigida := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"} {
			if !slices.Contains(comando.Ambiente, exigida) {
				t.Errorf("git %v sem %s: %v", comando.Argumentos, exigida, comando.Ambiente)
			}
		}
		for _, variavel := range comando.Ambiente {
			if strings.Contains(strings.ToUpper(variavel), "TOKEN") {
				t.Errorf("nenhum token entra no clone: %s", variavel)
			}
		}
	}
}
