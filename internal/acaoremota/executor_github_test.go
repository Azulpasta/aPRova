package acaoremota

import (
	"context"
	"errors"
	"testing"

	"github.com/Azulpasta/aPRova/internal/platform"
)

type reviewPublicada struct {
	installationID int64
	dono           string
	repositorio    string
	numero         int
	review         platform.Review
}

type publicadorFalso struct {
	publicadas []reviewPublicada
	erro       error
}

func (p *publicadorFalso) EnviarReview(_ context.Context, installationID int64, dono, repositorio string, numero int, review platform.Review) error {
	p.publicadas = append(p.publicadas, reviewPublicada{installationID, dono, repositorio, numero, review})
	return p.erro
}

type reenfileiradorFalso struct {
	reenfileiradas []AcaoAutorizada
}

func (r *reenfileiradorFalso) Reenfileirar(_ context.Context, acao AcaoAutorizada) error {
	r.reenfileiradas = append(r.reenfileiradas, acao)
	return nil
}

func acaoAutorizadaDeTeste(tipo Tipo, justificativa string) AcaoAutorizada {
	return AcaoAutorizada{
		Pedido: Pedido{
			IDAcao:         "acao-1",
			Tipo:           tipo,
			UsuarioSlack:   "U-ESCRITOR",
			Repositorio:    "Azulpasta/aPRova",
			NumeroPR:       42,
			InstallationID: 7001,
			Justificativa:  justificativa,
		},
		LoginGitHub: "erin",
	}
}

func TestExecutorAprovaNomeandoOHumanoQueDecidiu(t *testing.T) {
	publicador := &publicadorFalso{}
	reenfileirador := &reenfileiradorFalso{}

	err := NovoExecutorNoGitHub(publicador, reenfileirador).Executar(t.Context(), acaoAutorizadaDeTeste(TipoAprovar, ""))

	if err != nil {
		t.Fatalf("aprovação não deveria falhar: %v", err)
	}
	esperada := reviewPublicada{7001, "Azulpasta", "aPRova", 42, platform.Review{
		Evento: platform.EventoAprovar,
		Corpo:  "Aprovado por @erin via Slack",
	}}
	if len(publicador.publicadas) != 1 || publicador.publicadas[0] != esperada {
		t.Errorf("reviews = %+v, esperava só %+v", publicador.publicadas, esperada)
	}
	if len(reenfileirador.reenfileiradas) != 0 {
		t.Error("aprovar não reenfileira análise")
	}
}

func TestExecutorSolicitaAlteracaoComAJustificativa(t *testing.T) {
	publicador := &publicadorFalso{}

	err := NovoExecutorNoGitHub(publicador, &reenfileiradorFalso{}).Executar(t.Context(),
		acaoAutorizadaDeTeste(TipoSolicitarAlteracao, "falta teste para o caso de timeout"))

	if err != nil {
		t.Fatalf("solicitação de alteração não deveria falhar: %v", err)
	}
	esperada := platform.Review{
		Evento: platform.EventoSolicitarAlteracao,
		Corpo:  "Alteração solicitada por @erin via Slack:\n\nfalta teste para o caso de timeout",
	}
	if len(publicador.publicadas) != 1 || publicador.publicadas[0].review != esperada {
		t.Errorf("reviews = %+v, esperava só %+v", publicador.publicadas, esperada)
	}
}

func TestExecutorReenfileiraAnaliseSemPublicarReview(t *testing.T) {
	publicador := &publicadorFalso{}
	reenfileirador := &reenfileiradorFalso{}
	acao := acaoAutorizadaDeTeste(TipoReexecutarAnalise, "")

	err := NovoExecutorNoGitHub(publicador, reenfileirador).Executar(t.Context(), acao)

	if err != nil {
		t.Fatalf("reanálise não deveria falhar: %v", err)
	}
	if len(reenfileirador.reenfileiradas) != 1 || reenfileirador.reenfileiradas[0] != acao {
		t.Errorf("reenfileiradas = %+v, esperava só %+v", reenfileirador.reenfileiradas, acao)
	}
	if len(publicador.publicadas) != 0 {
		t.Error("reanálise não publica review")
	}
}

func TestExecutorDevolveAFalhaDoGitHub(t *testing.T) {
	publicador := &publicadorFalso{erro: platform.ErrGitHubIndisponivel}

	err := NovoExecutorNoGitHub(publicador, &reenfileiradorFalso{}).Executar(t.Context(), acaoAutorizadaDeTeste(TipoAprovar, ""))

	if !errors.Is(err, platform.ErrGitHubIndisponivel) {
		t.Errorf("a falha do GitHub precisa chegar a quem chamou, obtive %v", err)
	}
}

func TestExecutorRecusaTipoSemExecucaoDefinida(t *testing.T) {
	publicador := &publicadorFalso{}

	err := NovoExecutorNoGitHub(publicador, &reenfileiradorFalso{}).Executar(t.Context(), acaoAutorizadaDeTeste("mesclar", ""))

	if !errors.Is(err, ErrTipoSemExecucao) {
		t.Errorf("esperava ErrTipoSemExecucao, obtive %v", err)
	}
	if len(publicador.publicadas) != 0 {
		t.Error("tipo desconhecido não pode publicar nada")
	}
}

func TestReenfileiradorIndisponivelSempreFalha(t *testing.T) {
	err := ReenfileiradorIndisponivel{}.Reenfileirar(t.Context(), acaoAutorizadaDeTeste(TipoReexecutarAnalise, ""))

	if !errors.Is(err, ErrReanaliseIndisponivel) {
		t.Errorf("enquanto o worker não existe, reanálise precisa falhar explicitamente, obtive %v", err)
	}
}
