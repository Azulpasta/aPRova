package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type resultadoComando struct {
	saida  string
	erros  string
	codigo int
}

type executorDeComando interface {
	executar(ctx context.Context, binario string, argumentos ...string) (resultadoComando, error)
}

type comandoDoSistema struct{}

func (comandoDoSistema) executar(ctx context.Context, binario string, argumentos ...string) (resultadoComando, error) {
	var saida, erros bytes.Buffer
	processo := exec.CommandContext(ctx, binario, argumentos...)
	processo.Stdout = &saida
	processo.Stderr = &erros

	err := processo.Run()
	resultado := resultadoComando{saida: saida.String(), erros: erros.String()}

	var saiuComErro *exec.ExitError
	if errors.As(err, &saiuComErro) && ctx.Err() == nil {
		resultado.codigo = saiuComErro.ExitCode()
		return resultado, nil
	}

	return resultado, err
}
