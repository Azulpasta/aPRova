package acaoremota

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/platform"
)

var momentoDeTeste = time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC)

type consultaDePermissao struct {
	installationID int64
	dono           string
	repositorio    string
	login          string
}

type permissoesFalsas struct {
	mutex     sync.Mutex
	porLogin  map[string]platform.Permissao
	erro      error
	consultas []consultaDePermissao
}

func (p *permissoesFalsas) PermissaoNoRepositorio(_ context.Context, installationID int64, dono, repositorio, login string) (platform.Permissao, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.consultas = append(p.consultas, consultaDePermissao{installationID, dono, repositorio, login})
	if p.erro != nil {
		return "", p.erro
	}
	return p.porLogin[login], nil
}

type autoriaFalsa struct {
	autor string
	erro  error
}

func (a *autoriaFalsa) AutorDoPullRequest(context.Context, int64, string, string, int) (string, error) {
	return a.autor, a.erro
}

type executorFalso struct {
	mutex      sync.Mutex
	executadas []AcaoAutorizada
	erro       error
}

func (e *executorFalso) Executar(_ context.Context, acao AcaoAutorizada) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.executadas = append(e.executadas, acao)
	return e.erro
}

type cenario struct {
	permissoes *permissoesFalsas
	autoria    *autoriaFalsa
	executor   *executorFalso
	tentativas *RegistroEmMemoria
	servico    *Servico
}

func novoCenario(t *testing.T) *cenario {
	t.Helper()

	c := &cenario{
		permissoes: &permissoesFalsas{porLogin: map[string]platform.Permissao{
			"dave":  platform.PermissaoRead,
			"erin":  platform.PermissaoWrite,
			"carol": platform.PermissaoAdmin,
		}},
		autoria:    &autoriaFalsa{autor: "carol"},
		executor:   &executorFalso{},
		tentativas: NovoRegistroEmMemoria(),
	}

	servico, err := NovoServico(OpcoesServico{
		Identidades: NovoResolvedorEmMemoria(map[string]string{
			"U-LEITOR":   "dave",
			"U-ESCRITOR": "erin",
			"U-AUTORA":   "carol",
		}),
		Permissoes: c.permissoes,
		Autoria:    c.autoria,
		Executor:   c.executor,
		Tentativas: c.tentativas,
		Reservas:   NovaReservaEmMemoria(),
		Agora:      func() time.Time { return momentoDeTeste },
	})
	if err != nil {
		t.Fatalf("montar o serviço: %v", err)
	}
	c.servico = servico

	return c
}

func corpoDoPedido(t *testing.T, campos map[string]any) []byte {
	t.Helper()

	pedido := map[string]any{
		"id_acao":         "acao-1",
		"tipo":            "aprovar",
		"usuario_slack":   "U-ESCRITOR",
		"repositorio":     "Azulpasta/aPRova",
		"numero_pr":       42,
		"installation_id": 7001,
	}
	for campo, valor := range campos {
		if valor == nil {
			delete(pedido, campo)
			continue
		}
		pedido[campo] = valor
	}

	corpo, err := json.Marshal(pedido)
	if err != nil {
		t.Fatalf("serializar pedido: %v", err)
	}
	return corpo
}

func (c *cenario) unicaTentativa(t *testing.T) Tentativa {
	t.Helper()

	tentativas := c.tentativas.Tentativas()
	if len(tentativas) != 1 {
		t.Fatalf("esperava exatamente uma tentativa registrada, há %d", len(tentativas))
	}
	return tentativas[0]
}

func TestServicoRecusaAprovacaoDeQuemSoLeORepositorio(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-LEITOR"}))

	if decisao != DecisaoRecusada {
		t.Fatalf("permissão read não aprova: esperava %q, obtive %q", DecisaoRecusada, decisao)
	}
	if len(c.executor.executadas) != 0 {
		t.Errorf("ação recusada não pode ser executada, foram %d execuções", len(c.executor.executadas))
	}

	tentativa := c.unicaTentativa(t)
	esperada := Tentativa{
		IDAcao:       "acao-1",
		Tipo:         TipoAprovar,
		UsuarioSlack: "U-LEITOR",
		LoginGitHub:  "dave",
		Repositorio:  "Azulpasta/aPRova",
		NumeroPR:     42,
		Decisao:      DecisaoRecusada,
		Motivo:       MotivoPermissaoInsuficiente,
		Em:           momentoDeTeste,
	}
	if tentativa != esperada {
		t.Errorf("registro da recusa\nobtido:   %+v\nesperado: %+v", tentativa, esperada)
	}
}

type resolvedorComErro struct{}

func (resolvedorComErro) LoginGitHub(context.Context, string) (string, error) {
	return "", errors.New("banco de vínculos fora do ar")
}

func TestServicoRecusaUsuarioSlackSemVinculo(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-DESCONHECIDO"}))

	if decisao != DecisaoRecusada {
		t.Fatalf("usuário sem vínculo precisa ser recusado, obtive %q", decisao)
	}
	if len(c.permissoes.consultas) != 0 {
		t.Error("sem login resolvido não há o que consultar no GitHub")
	}

	tentativa := c.unicaTentativa(t)
	if tentativa.Motivo != MotivoSemVinculo || tentativa.LoginGitHub != "" || tentativa.UsuarioSlack != "U-DESCONHECIDO" {
		t.Errorf("registro da recusa por falta de vínculo errado: %+v", tentativa)
	}
}

func TestServicoRecusaQuandoOResolvedorFalha(t *testing.T) {
	c := novoCenario(t)
	c.servico.identidades = resolvedorComErro{}

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoRecusada {
		t.Fatalf("falha ao resolver identidade não pode liberar a ação, obtive %q", decisao)
	}
	if motivo := c.unicaTentativa(t).Motivo; motivo != MotivoFalhaNaIdentidade {
		t.Errorf("motivo = %q, esperava %q", motivo, MotivoFalhaNaIdentidade)
	}
}

func TestServicoConsultaAPermissaoDoLoginResolvidoNoRepositorioPedido(t *testing.T) {
	c := novoCenario(t)

	c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	esperada := []consultaDePermissao{{installationID: 7001, dono: "Azulpasta", repositorio: "aPRova", login: "erin"}}
	if !slices.Equal(c.permissoes.consultas, esperada) {
		t.Errorf("consultas = %+v, esperava %+v", c.permissoes.consultas, esperada)
	}
}

func TestServicoRecusaQuandoAConsultaDePermissaoFalha(t *testing.T) {
	c := novoCenario(t)
	c.permissoes.erro = platform.ErrGitHubIndisponivel

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoRecusada {
		t.Fatalf("falha na verificação nunca libera a ação, obtive %q", decisao)
	}
	if len(c.executor.executadas) != 0 {
		t.Error("nada pode ser executado quando a permissão não foi confirmada")
	}
	if motivo := c.unicaTentativa(t).Motivo; motivo != MotivoFalhaNaVerificacao {
		t.Errorf("motivo = %q, esperava %q", motivo, MotivoFalhaNaVerificacao)
	}
}

func TestServicoRecusaAutorAprovandoOProprioPR(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-AUTORA"}))

	if decisao != DecisaoRecusada {
		t.Fatalf("a autora é admin mas não pode aprovar o próprio PR, obtive %q", decisao)
	}
	if motivo := c.unicaTentativa(t).Motivo; motivo != MotivoAutorDoProprioPR {
		t.Errorf("motivo = %q, esperava %q", motivo, MotivoAutorDoProprioPR)
	}
}

func TestServicoRecusaAprovacaoQuandoNaoConsegueDescobrirOAutor(t *testing.T) {
	c := novoCenario(t)
	c.autoria.erro = platform.ErrGitHubIndisponivel

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoRecusada {
		t.Fatalf("sem saber o autor não dá para garantir que ele não se aprova, obtive %q", decisao)
	}
	if motivo := c.unicaTentativa(t).Motivo; motivo != MotivoFalhaNaVerificacao {
		t.Errorf("motivo = %q, esperava %q", motivo, MotivoFalhaNaVerificacao)
	}
}

type resolvedorSemLogin struct{}

func (resolvedorSemLogin) LoginGitHub(context.Context, string) (string, error) {
	return "", nil
}

type reservaComErro struct{}

func (reservaComErro) Reservar(context.Context, string) (bool, error) {
	return false, errors.New("armazenamento de reservas fora do ar")
}

func TestServicoTrataLoginVazioComoFaltaDeVinculo(t *testing.T) {
	c := novoCenario(t)
	c.servico.identidades = resolvedorSemLogin{}

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoRecusada || c.unicaTentativa(t).Motivo != MotivoSemVinculo {
		t.Errorf("login vazio não identifica ninguém, obtive %q", decisao)
	}
}

func TestServicoExecutaAprovacaoDeQuemTemEscritaEmPRDeOutraPessoa(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoAceita {
		t.Fatalf("write aprovando PR de outra pessoa precisa ser aceito, obtive %q", decisao)
	}

	esperada := AcaoAutorizada{
		Pedido: Pedido{
			IDAcao:         "acao-1",
			Tipo:           TipoAprovar,
			UsuarioSlack:   "U-ESCRITOR",
			Repositorio:    "Azulpasta/aPRova",
			NumeroPR:       42,
			InstallationID: 7001,
		},
		LoginGitHub: "erin",
	}
	if len(c.executor.executadas) != 1 || c.executor.executadas[0] != esperada {
		t.Errorf("execuções = %+v, esperava só %+v", c.executor.executadas, esperada)
	}

	tentativa := c.unicaTentativa(t)
	if tentativa.Decisao != DecisaoAceita || tentativa.Motivo != MotivoAutorizada || tentativa.LoginGitHub != "erin" {
		t.Errorf("registro da aceitação errado: %+v", tentativa)
	}
}

func TestServicoPermiteReanaliseSemConsultarOAutor(t *testing.T) {
	c := novoCenario(t)
	c.autoria.erro = platform.ErrGitHubIndisponivel

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"tipo": "reexecutar_analise"}))

	if decisao != DecisaoAceita {
		t.Errorf("reanálise não depende do autor do PR, obtive %q", decisao)
	}
}

func TestServicoNaoExecutaDuasVezesOMesmoIDAcao(t *testing.T) {
	c := novoCenario(t)
	corpo := corpoDoPedido(t, nil)

	primeira := c.servico.Processar(t.Context(), corpo)
	segunda := c.servico.Processar(t.Context(), corpo)

	if primeira != DecisaoAceita || segunda != DecisaoRepetida {
		t.Fatalf("esperava aceita e depois repetida, obtive %q e %q", primeira, segunda)
	}
	if len(c.executor.executadas) != 1 {
		t.Errorf("o mesmo id_acao executou %d vezes", len(c.executor.executadas))
	}

	tentativas := c.tentativas.Tentativas()
	if len(tentativas) != 2 || tentativas[1].Decisao != DecisaoRepetida || tentativas[1].Motivo != MotivoAcaoRepetida {
		t.Errorf("a repetição também precisa ficar registrada: %+v", tentativas)
	}
}

func TestServicoExecutaUmaUnicaVezEmCliquesConcorrentes(t *testing.T) {
	c := novoCenario(t)
	corpo := corpoDoPedido(t, nil)

	var aceitas sync.Map
	var grupo sync.WaitGroup
	for indice := range 20 {
		grupo.Go(func() {
			aceitas.Store(indice, c.servico.Processar(t.Context(), corpo))
		})
	}
	grupo.Wait()

	quantidadeAceita := 0
	aceitas.Range(func(_, decisao any) bool {
		if decisao == DecisaoAceita {
			quantidadeAceita++
		}
		return true
	})

	if quantidadeAceita != 1 || len(c.executor.executadas) != 1 {
		t.Errorf("cliques concorrentes: %d aceitas e %d execuções, esperava 1 e 1",
			quantidadeAceita, len(c.executor.executadas))
	}
}

func TestServicoRecusaNaoConsomeOIDAcao(t *testing.T) {
	c := novoCenario(t)

	recusada := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"usuario_slack": "U-LEITOR"}))
	aceita := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if recusada != DecisaoRecusada || aceita != DecisaoAceita {
		t.Errorf("recusa não produz efeito e não reserva o id, obtive %q e %q", recusada, aceita)
	}
}

func TestServicoInformaFalhaQuandoAExecucaoFalha(t *testing.T) {
	c := novoCenario(t)
	c.executor.erro = platform.ErrGitHubIndisponivel

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoFalha {
		t.Fatalf("execução que falhou não pode ser dada como aceita, obtive %q", decisao)
	}
	if tentativa := c.unicaTentativa(t); tentativa.Decisao != DecisaoFalha || tentativa.Motivo != MotivoFalhaNaExecucao {
		t.Errorf("registro da falha errado: %+v", tentativa)
	}
}

func TestServicoNaoExecutaQuandoAReservaFalha(t *testing.T) {
	c := novoCenario(t)
	c.servico.reservas = reservaComErro{}

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, nil))

	if decisao != DecisaoFalha {
		t.Fatalf("sem reserva não há garantia contra execução dupla, obtive %q", decisao)
	}
	if len(c.executor.executadas) != 0 {
		t.Error("nada pode ser executado sem a reserva do id_acao")
	}
	if motivo := c.unicaTentativa(t).Motivo; motivo != MotivoFalhaNaReserva {
		t.Errorf("motivo = %q, esperava %q", motivo, MotivoFalhaNaReserva)
	}
}
