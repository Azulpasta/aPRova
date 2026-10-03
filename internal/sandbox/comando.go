package sandbox

import (
	"context"
	"errors"
	"os/exec"
)

type resultadoComando struct {
	saida  string
	codigo int
}

type executorDeComando interface {
	executar(ctx context.Context, binario string, argumentos ...string) (resultadoComando, error)
}

type comandoDoSistema struct{}

func (comandoDoSistema) executar(ctx context.Context, binario string, argumentos ...string) (resultadoComando, error) {
	saida, err := exec.CommandContext(ctx, binario, argumentos...).CombinedOutput()

	var saiuComErro *exec.ExitError
	if errors.As(err, &saiuComErro) && ctx.Err() == nil {
		return resultadoComando{saida: string(saida), codigo: saiuComErro.ExitCode()}, nil
	}

	return resultadoComando{saida: string(saida)}, err
}
