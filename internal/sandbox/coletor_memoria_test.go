package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func resultadoDeExemplo() ResultadoSandbox {
	return ResultadoSandbox{Compilou: true, SaidaBruta: "ok", DuracaoMS: 1200, TipoFalha: FalhaNenhuma}
}

func TestAguardarRecebeOQueGravarEscreveu(t *testing.T) {
	coletor := NovoColetorEmMemoria()

	if err := coletor.Gravar(t.Context(), "task-run-1", resultadoDeExemplo()); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	obtido, err := coletor.Aguardar(t.Context(), "task-run-1")
	if err != nil {
		t.Fatalf("aguardar: %v", err)
	}
	if obtido != resultadoDeExemplo() {
		t.Errorf("esperava %+v, obtive %+v", resultadoDeExemplo(), obtido)
	}
}

func TestAguardarBloqueiaAteOResultadoSerGravado(t *testing.T) {
	coletor := NovoColetorEmMemoria()

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = coletor.Gravar(context.Background(), "task-run-1", resultadoDeExemplo())
	}()

	obtido, err := coletor.Aguardar(t.Context(), "task-run-1")
	if err != nil {
		t.Fatalf("aguardar: %v", err)
	}
	if obtido != resultadoDeExemplo() {
		t.Errorf("esperava o resultado gravado depois, obtive %+v", obtido)
	}
}

func TestAguardarRespeitaCancelamentoDoContexto(t *testing.T) {
	coletor := NovoColetorEmMemoria()
	ctx, cancelar := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancelar()

	inicio := time.Now()
	_, err := coletor.Aguardar(ctx, "task-run-que-nunca-grava")

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("esperava o erro do contexto, obtive %v", err)
	}
	if decorrido := time.Since(inicio); decorrido > time.Second {
		t.Errorf("Aguardar deveria voltar junto com o prazo, levou %v", decorrido)
	}
}

func TestResultadosDeTaskRunsDiferentesNaoSeMisturam(t *testing.T) {
	coletor := NovoColetorEmMemoria()
	falhou := ResultadoSandbox{TipoFalha: FalhaTeste}

	_ = coletor.Gravar(t.Context(), "task-run-a", resultadoDeExemplo())
	_ = coletor.Gravar(t.Context(), "task-run-b", falhou)

	obtido, err := coletor.Aguardar(t.Context(), "task-run-b")
	if err != nil {
		t.Fatalf("aguardar: %v", err)
	}
	if obtido != falhou {
		t.Errorf("task-run-b recebeu o resultado de outra execução: %+v", obtido)
	}
}

func TestGravarDuasVezesOMesmoTaskRunERecusado(t *testing.T) {
	coletor := NovoColetorEmMemoria()
	_ = coletor.Gravar(t.Context(), "task-run-1", resultadoDeExemplo())

	err := coletor.Gravar(t.Context(), "task-run-1", ResultadoSandbox{TipoFalha: FalhaTeste})

	if !errors.Is(err, ErrResultadoJaGravado) {
		t.Errorf("o primeiro resultado é o que vale; esperava ErrResultadoJaGravado, obtive %v", err)
	}
	if obtido, _ := coletor.Aguardar(t.Context(), "task-run-1"); obtido != resultadoDeExemplo() {
		t.Errorf("a segunda gravação não pode sobrescrever a primeira: %+v", obtido)
	}
}
