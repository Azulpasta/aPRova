package sandbox

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// ErrPedidoInvalido indica pedido rejeitado antes de qualquer execução.
var ErrPedidoInvalido = errors.New("sandbox: pedido de execução inválido")

var validador = novoValidador()

func novoValidador() *validator.Validate {
	validacao := validator.New(validator.WithRequiredStructEnabled())
	if err := validacao.RegisterValidation("diretorio_do_projeto", diretorioDoProjetoValido); err != nil {
		panic(err)
	}
	return validacao
}

func diretorioDoProjetoValido(campo validator.FieldLevel) bool {
	return ValidarDiretorioDoProjeto(campo.Field().String()) == nil
}

func validarPedido(pedido PedidoExecucao) error {
	if err := validador.Struct(pedido); err != nil {
		return fmt.Errorf("%w: %w", ErrPedidoInvalido, err)
	}
	return nil
}
