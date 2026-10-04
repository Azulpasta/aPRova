package acaoremota

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResolvedorEmMemoriaDevolveOLoginVinculado(t *testing.T) {
	resolvedor := NovoResolvedorEmMemoria(map[string]string{"U01": "alice"})

	login, err := resolvedor.LoginGitHub(t.Context(), "U01")

	if err != nil || login != "alice" {
		t.Errorf("esperava alice sem erro, obtive %q e %v", login, err)
	}
}

func TestResolvedorEmMemoriaSinalizaUsuarioSemVinculo(t *testing.T) {
	resolvedor := NovoResolvedorEmMemoria(map[string]string{"U01": "alice"})

	_, err := resolvedor.LoginGitHub(t.Context(), "U02")

	if !errors.Is(err, ErrSemVinculo) {
		t.Errorf("esperava ErrSemVinculo, obtive %v", err)
	}
}

func TestResolvedorEmMemoriaNaoEnxergaMudancaNoMapaOriginal(t *testing.T) {
	vinculos := map[string]string{"U01": "alice"}
	resolvedor := NovoResolvedorEmMemoria(vinculos)

	vinculos["U02"] = "mallory"

	if _, err := resolvedor.LoginGitHub(t.Context(), "U02"); !errors.Is(err, ErrSemVinculo) {
		t.Error("o resolvedor precisa guardar a própria cópia do mapa")
	}
}

func TestReservaEmMemoriaAceitaCadaIDUmaUnicaVez(t *testing.T) {
	reserva := NovaReservaEmMemoria()

	primeira, err := reserva.Reservar(t.Context(), "acao-1")
	if err != nil || !primeira {
		t.Fatalf("a primeira reserva precisa ser aceita, obtive %v e %v", primeira, err)
	}

	segunda, err := reserva.Reservar(t.Context(), "acao-1")
	if err != nil || segunda {
		t.Errorf("a segunda reserva do mesmo id precisa ser negada, obtive %v e %v", segunda, err)
	}

	outra, err := reserva.Reservar(t.Context(), "acao-2")
	if err != nil || !outra {
		t.Errorf("outro id precisa ser aceito, obtive %v e %v", outra, err)
	}
}

func TestReservaEmMemoriaAceitaUmUnicoVencedorEntreConcorrentes(t *testing.T) {
	reserva := NovaReservaEmMemoria()

	var vencedores atomic.Int32
	var grupo sync.WaitGroup
	for range 50 {
		grupo.Go(func() {
			if reservada, _ := reserva.Reservar(t.Context(), "clique-duplo"); reservada {
				vencedores.Add(1)
			}
		})
	}
	grupo.Wait()

	if vencedores.Load() != 1 {
		t.Errorf("exatamente uma reserva concorrente pode vencer, venceram %d", vencedores.Load())
	}
}

func TestRegistroEmMemoriaDevolveCopiaNaOrdemDeChegada(t *testing.T) {
	registro := NovoRegistroEmMemoria()
	_ = registro.Registrar(t.Context(), Tentativa{IDAcao: "a"})
	_ = registro.Registrar(t.Context(), Tentativa{IDAcao: "b"})

	tentativas := registro.Tentativas()
	tentativas[0].IDAcao = "adulterada"

	novamente := registro.Tentativas()
	if len(novamente) != 2 || novamente[0].IDAcao != "a" || novamente[1].IDAcao != "b" {
		t.Errorf("registro alterado por fora ou fora de ordem: %+v", novamente)
	}
}
