package conteiner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

// ErrClone indica que o repositório não pôde ser posicionado no SHA pedido.
var ErrClone = errors.New("sandbox: clone falhou")

var ambienteDoGit = []string{
	"PATH=/usr/local/bin:/usr/bin:/bin",
	"HOME=/nonexistent",
	"GIT_TERMINAL_PROMPT=0",
	"GIT_CONFIG_NOSYSTEM=1",
	"GIT_CONFIG_GLOBAL=/dev/null",
}

// Clonador obtém o repositório no SHA exato, sem nenhuma credencial.
type Clonador struct {
	Rodador processo.Rodador
}

// Clonar cria o destino, busca apenas o commit pedido e posiciona o checkout
// nele, conferindo depois que HEAD é exatamente o SHA pedido.
func (c Clonador) Clonar(ctx context.Context, repositorio, sha, destino string) error {
	etapas := [][]string{
		{"init", "-q", destino},
		{"-C", destino, "remote", "add", "origin", repositorio},
		{"-C", destino, "fetch", "-q", "--depth", "1", "--no-tags", "origin", sha},
		{"-C", destino, "checkout", "-q", "--detach", "FETCH_HEAD"},
	}

	for _, argumentos := range etapas {
		if _, err := c.git(ctx, argumentos...); err != nil {
			return err
		}
	}

	head, err := c.git(ctx, "-C", destino, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != sha {
		return fmt.Errorf("%w: HEAD %s diverge do SHA pedido %s", ErrClone, head, sha)
	}
	return nil
}

func (c Clonador) git(ctx context.Context, argumentos ...string) (string, error) {
	execucao := c.Rodador.Rodar(ctx, processo.Comando{Binario: "git", Argumentos: argumentos, Ambiente: ambienteDoGit})

	etapa := "git " + argumentos[subcomandoDoGit(argumentos)]
	switch {
	case execucao.Erro != nil:
		return "", fmt.Errorf("%w: %s não pôde ser iniciado: %w", ErrClone, etapa, execucao.Erro)
	case execucao.PrazoEsgotado:
		return "", fmt.Errorf("%w: %s excedeu o prazo", ErrClone, etapa)
	case execucao.CodigoSaida != 0:
		return "", fmt.Errorf("%w: %s saiu com código %d: %s", ErrClone, etapa, execucao.CodigoSaida, strings.TrimSpace(string(execucao.Saida)))
	}

	return strings.TrimSpace(string(execucao.Saida)), nil
}

func subcomandoDoGit(argumentos []string) int {
	if argumentos[0] == "-C" {
		return 2
	}
	return 0
}
