package sandbox

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

// ErrPedidoInvalido indica pedido rejeitado antes de qualquer execução.
var ErrPedidoInvalido = errors.New("sandbox: pedido de execução inválido")

var validador = validator.New(validator.WithRequiredStructEnabled())

func validarPedido(pedido PedidoExecucao) error {
	if err := validador.Struct(pedido); err != nil {
		return fmt.Errorf("%w: %w", ErrPedidoInvalido, err)
	}
	return nil
}
