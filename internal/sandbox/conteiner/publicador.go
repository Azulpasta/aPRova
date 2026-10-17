package conteiner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

const (
	destinoSaidaPadrao = "stdout"
	destinoColetor     = "coletor"
)

var (
	// ErrSaidaDesconhecida indica APROVA_SAIDA com valor não suportado.
	ErrSaidaDesconhecida = errors.New("sandbox: APROVA_SAIDA desconhecida")

	// ErrColetorIndisponivel indica APROVA_SAIDA=coletor sem coletor concreto.
	ErrColetorIndisponivel = errors.New("sandbox: APROVA_SAIDA=coletor ainda não tem implementação")
)

// PublicadorResultado entrega o resultado da execução a quem o aguarda.
type PublicadorResultado interface {
	Publicar(ctx context.Context, taskRunID string, resultado sandbox.ResultadoSandbox) error
}

// PublicadorNaSaida escreve o resultado como uma única linha JSON. Deve ser a
// única coisa que o executor de dentro escreve no seu stdout.
type PublicadorNaSaida struct {
	destino io.Writer
}

// NovoPublicadorNaSaida constrói o publicador que escreve no destino informado.
func NovoPublicadorNaSaida(destino io.Writer) *PublicadorNaSaida {
	return &PublicadorNaSaida{destino: destino}
}

// Publicar escreve o resultado em JSON seguido de uma quebra de linha.
func (p *PublicadorNaSaida) Publicar(_ context.Context, _ string, resultado sandbox.ResultadoSandbox) error {
	linha, err := json.Marshal(resultado)
	if err != nil {
		return fmt.Errorf("serializar resultado: %w", err)
	}

	if _, err := p.destino.Write(append(linha, '\n')); err != nil {
		return fmt.Errorf("escrever resultado: %w", err)
	}
	return nil
}

// PublicadorNoColetor grava o resultado pelo ColetorResultado.
type PublicadorNoColetor struct {
	coletor sandbox.ColetorResultado
}

// NovoPublicadorNoColetor constrói o publicador sobre o coletor informado.
func NovoPublicadorNoColetor(coletor sandbox.ColetorResultado) *PublicadorNoColetor {
	return &PublicadorNoColetor{coletor: coletor}
}

// Publicar grava o resultado associado ao task_run.
func (p *PublicadorNoColetor) Publicar(ctx context.Context, taskRunID string, resultado sandbox.ResultadoSandbox) error {
	return p.coletor.Gravar(ctx, taskRunID, resultado)
}

// EscolherPublicador escolhe o publicador pelo valor de APROVA_SAIDA. Vazio
// equivale a stdout; coletor exige um ColetorResultado concreto.
func EscolherPublicador(destino string, saida io.Writer, coletor sandbox.ColetorResultado) (PublicadorResultado, error) {
	switch destino {
	case "", destinoSaidaPadrao:
		return NovoPublicadorNaSaida(saida), nil
	case destinoColetor:
		if coletor == nil {
			return nil, ErrColetorIndisponivel
		}
		return NovoPublicadorNoColetor(coletor), nil
	default:
		return nil, fmt.Errorf("%w: %q, use stdout ou coletor", ErrSaidaDesconhecida, destino)
	}
}
