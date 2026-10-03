package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrResultadoJaGravado indica uma segunda gravação para o mesmo task_run.
var ErrResultadoJaGravado = errors.New("sandbox: resultado já gravado para este task_run")

type resultadoPendente struct {
	pronto    chan struct{}
	resultado ResultadoSandbox
	gravado   bool
}

// ColetorEmMemoria guarda resultados apenas na memória do processo. Atende
// testes e o executor local; não sobrevive a reinício nem cruza processos.
type ColetorEmMemoria struct {
	mutex     sync.Mutex
	pendentes map[string]*resultadoPendente
}

// NovoColetorEmMemoria constrói um coletor vazio.
func NovoColetorEmMemoria() *ColetorEmMemoria {
	return &ColetorEmMemoria{pendentes: map[string]*resultadoPendente{}}
}

// Gravar registra o resultado do task_run e libera quem o aguarda. Uma
// segunda gravação para o mesmo task_run é recusada.
func (c *ColetorEmMemoria) Gravar(_ context.Context, taskRunID string, resultado ResultadoSandbox) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	pendente := c.pendenteDe(taskRunID)
	if pendente.gravado {
		return fmt.Errorf("%w: %s", ErrResultadoJaGravado, taskRunID)
	}

	pendente.resultado = resultado
	pendente.gravado = true
	close(pendente.pronto)

	return nil
}

// Aguardar bloqueia até o resultado do task_run ser gravado ou o contexto
// terminar, o que vier primeiro.
func (c *ColetorEmMemoria) Aguardar(ctx context.Context, taskRunID string) (ResultadoSandbox, error) {
	c.mutex.Lock()
	pendente := c.pendenteDe(taskRunID)
	c.mutex.Unlock()

	select {
	case <-pendente.pronto:
		return pendente.resultado, nil
	case <-ctx.Done():
		return ResultadoSandbox{}, ctx.Err()
	}
}

func (c *ColetorEmMemoria) pendenteDe(taskRunID string) *resultadoPendente {
	pendente, existe := c.pendentes[taskRunID]
	if !existe {
		pendente = &resultadoPendente{pronto: make(chan struct{})}
		c.pendentes[taskRunID] = pendente
	}
	return pendente
}
