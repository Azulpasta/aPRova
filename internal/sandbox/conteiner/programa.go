package conteiner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

const (
	codigoResultadoEntregue   = 0
	codigoEntregaImpossivel   = 1
	subdiretorioDoRepositorio = "repositorio"
)

// ExecutorDeLinguagem roda build e testes de uma linguagem no repositório já
// clonado. prazoFinal é o instante em que o resultado já precisa ter sido
// entregue.
type ExecutorDeLinguagem interface {
	Executar(ctx context.Context, diretorio string, prazoFinal time.Time) sandbox.ResultadoSandbox
}

// Programa é o executor de dentro do container: lê o pedido do ambiente,
// clona o repositório, roda a linguagem e entrega o resultado.
type Programa struct {
	LerVariavel         func(string) string
	Saida               io.Writer
	Coletor             sandbox.ColetorResultado
	Rodador             processo.Rodador
	DiretorioDeTrabalho string
	Linguagens          map[sandbox.Linguagem]ExecutorDeLinguagem
	Agora               func() time.Time
}

// Executar roda o pedido e devolve o código de saída do processo: zero quando
// o resultado foi entregue, qualquer que seja ele, e um quando nem a entrega
// foi possível. Problemas de configuração viram resultado de infraestrutura.
func (p Programa) Executar(ctx context.Context) int {
	inicio := p.Agora()
	taskRunID := p.LerVariavel("APROVA_TASK_RUN_ID")

	publicador, err := EscolherPublicador(p.LerVariavel("APROVA_SAIDA"), p.Saida, p.Coletor)
	if err != nil {
		return p.entregar(ctx, NovoPublicadorNaSaida(p.Saida), taskRunID, inicio, falhaDeInfraestrutura(err))
	}

	return p.entregar(ctx, publicador, taskRunID, inicio, p.rodarPedido(ctx, inicio))
}

func (p Programa) rodarPedido(ctx context.Context, inicio time.Time) sandbox.ResultadoSandbox {
	ambiente, err := LerAmbiente(p.LerVariavel)
	if err != nil {
		return falhaDeInfraestrutura(err)
	}

	linguagem, suportada := p.Linguagens[ambiente.Linguagem]
	if !suportada {
		return falhaDeInfraestrutura(fmt.Errorf("linguagem %s ainda não suportada pelo sandbox", ambiente.Linguagem))
	}

	prazoFinal := inicio.Add(ambiente.Prazo)
	ctx, cancelar := context.WithDeadline(ctx, prazoFinal)
	defer cancelar()

	diretorio := filepath.Join(p.DiretorioDeTrabalho, subdiretorioDoRepositorio)
	if err := (Clonador{Rodador: p.Rodador}).Clonar(ctx, ambiente.Repositorio, ambiente.SHA, diretorio); err != nil {
		return falhaDeInfraestrutura(err)
	}

	return linguagem.Executar(ctx, diretorio, prazoFinal)
}

func (p Programa) entregar(ctx context.Context, publicador PublicadorResultado, taskRunID string, inicio time.Time, resultado sandbox.ResultadoSandbox) int {
	resultado.DuracaoMS = p.Agora().Sub(inicio).Milliseconds()
	resultado.SaidaBruta = sandbox.LimitarSaida(resultado.SaidaBruta)

	slog.Info("execução concluída", "task_run_id", taskRunID, "tipo_falha", resultado.TipoFalha,
		"testes_passaram", resultado.TestesPassaram, "testes_falharam", resultado.TestesFalharam)

	if err := publicador.Publicar(context.WithoutCancel(ctx), taskRunID, resultado); err != nil {
		slog.Error("resultado não pôde ser entregue", "task_run_id", taskRunID, "erro", err)
		return codigoEntregaImpossivel
	}
	return codigoResultadoEntregue
}

func falhaDeInfraestrutura(causa error) sandbox.ResultadoSandbox {
	slog.Error("execução sem resultado do PR", "erro", causa)
	return sandbox.ResultadoSandbox{SaidaBruta: "aprova: " + causa.Error() + "\n", TipoFalha: sandbox.FalhaInfraestrutura}
}
