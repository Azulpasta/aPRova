package acaoremota

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrSemVinculo indica usuário do Slack que não tem login do GitHub associado.
var ErrSemVinculo = errors.New("acaoremota: usuário do slack sem vínculo com o github")

// ErrDependenciaAusente indica serviço construído sem uma das dependências.
var ErrDependenciaAusente = errors.New("acaoremota: dependência ausente")

var nivelDoDiarioPorDecisao = map[Decisao]slog.Level{
	DecisaoAceita:   slog.LevelInfo,
	DecisaoRepetida: slog.LevelInfo,
	DecisaoRecusada: slog.LevelWarn,
	DecisaoInvalida: slog.LevelWarn,
	DecisaoFalha:    slog.LevelError,
}

// OpcoesServico reúne as dependências do serviço de ações remotas.
type OpcoesServico struct {
	Identidades ResolvedorIdentidade
	Permissoes  ConsultorPermissao
	Autoria     ConsultorAutoria
	Executor    ExecutorAcao
	Tentativas  RegistroTentativas
	Reservas    ReservaDeAcoes
	Agora       func() time.Time
}

// Servico é o ponto único por onde passa toda ação pedida por canal externo.
type Servico struct {
	identidades ResolvedorIdentidade
	permissoes  ConsultorPermissao
	autoria     ConsultorAutoria
	executor    ExecutorAcao
	tentativas  RegistroTentativas
	reservas    ReservaDeAcoes
	politica    Politica
	agora       func() time.Time
}

type desfecho struct {
	pedido  Pedido
	login   string
	decisao Decisao
	motivo  Motivo
	causa   error
}

// NovoServico constrói o serviço a partir das dependências informadas. Todas
// são obrigatórias, exceto o relógio, cujo padrão é time.Now.
func NovoServico(opcoes OpcoesServico) (*Servico, error) {
	if err := exigirDependencias(opcoes); err != nil {
		return nil, err
	}

	if opcoes.Agora == nil {
		opcoes.Agora = time.Now
	}

	return &Servico{
		identidades: opcoes.Identidades,
		permissoes:  opcoes.Permissoes,
		autoria:     opcoes.Autoria,
		executor:    opcoes.Executor,
		tentativas:  opcoes.Tentativas,
		reservas:    opcoes.Reservas,
		politica:    NovaPolitica(),
		agora:       opcoes.Agora,
	}, nil
}

// Processar interpreta o corpo do pedido, autoriza, executa e registra a
// tentativa, devolvendo a decisão tomada.
func (s *Servico) Processar(ctx context.Context, corpo []byte) Decisao {
	pedido, err := interpretarPedido(corpo)
	if err != nil {
		return s.concluir(ctx, desfecho{pedido: pedido, decisao: DecisaoInvalida, motivo: MotivoCorpoInvalido, causa: err})
	}

	resultado := s.autorizar(ctx, pedido)
	if resultado.decisao == DecisaoAceita {
		resultado = s.efetivar(ctx, resultado)
	}

	return s.concluir(ctx, resultado)
}

func (s *Servico) autorizar(ctx context.Context, pedido Pedido) desfecho {
	login, err := s.identidades.LoginGitHub(ctx, pedido.UsuarioSlack)
	if errors.Is(err, ErrSemVinculo) || (err == nil && login == "") {
		return desfecho{pedido: pedido, decisao: DecisaoRecusada, motivo: MotivoSemVinculo}
	}
	if err != nil {
		return desfecho{pedido: pedido, decisao: DecisaoRecusada, motivo: MotivoFalhaNaIdentidade, causa: err}
	}

	situacao, err := s.levantarSituacao(ctx, pedido, login)
	if err != nil {
		return desfecho{pedido: pedido, login: login, decisao: DecisaoRecusada, motivo: MotivoFalhaNaVerificacao, causa: err}
	}

	veredito := s.politica.Avaliar(situacao)
	if !veredito.Autorizada {
		return desfecho{pedido: pedido, login: login, decisao: DecisaoRecusada, motivo: veredito.Motivo}
	}

	return desfecho{pedido: pedido, login: login, decisao: DecisaoAceita, motivo: MotivoAutorizada}
}

func (s *Servico) efetivar(ctx context.Context, autorizado desfecho) desfecho {
	reservada, err := s.reservas.Reservar(ctx, autorizado.pedido.IDAcao)
	if err != nil {
		return autorizado.com(DecisaoFalha, MotivoFalhaNaReserva, err)
	}
	if !reservada {
		return autorizado.com(DecisaoRepetida, MotivoAcaoRepetida, nil)
	}

	acao := AcaoAutorizada{Pedido: autorizado.pedido, LoginGitHub: autorizado.login}
	if err := s.executor.Executar(ctx, acao); err != nil {
		return autorizado.com(DecisaoFalha, MotivoFalhaNaExecucao, err)
	}

	return autorizado
}

func (s *Servico) levantarSituacao(ctx context.Context, pedido Pedido, login string) (Situacao, error) {
	dono, repositorio := pedido.donoERepositorio()

	permissao, err := s.permissoes.PermissaoNoRepositorio(ctx, pedido.InstallationID, dono, repositorio, login)
	if err != nil {
		return Situacao{}, err
	}

	situacao := Situacao{Tipo: pedido.Tipo, Permissao: permissao, Login: login}
	if pedido.Tipo != TipoAprovar {
		return situacao, nil
	}

	situacao.AutorDoPR, err = s.autoria.AutorDoPullRequest(ctx, pedido.InstallationID, dono, repositorio, pedido.NumeroPR)
	return situacao, err
}

func (s *Servico) concluir(ctx context.Context, resultado desfecho) Decisao {
	tentativa := Tentativa{
		IDAcao:       resultado.pedido.IDAcao,
		Tipo:         resultado.pedido.Tipo,
		UsuarioSlack: resultado.pedido.UsuarioSlack,
		LoginGitHub:  resultado.login,
		Repositorio:  resultado.pedido.Repositorio,
		NumeroPR:     resultado.pedido.NumeroPR,
		Decisao:      resultado.decisao,
		Motivo:       resultado.motivo,
		Em:           s.agora(),
	}

	registrarNoDiario(ctx, tentativa, resultado.causa)

	if err := s.tentativas.Registrar(ctx, tentativa); err != nil {
		slog.ErrorContext(ctx, "falha ao registrar tentativa de ação remota", "id_acao", tentativa.IDAcao, "erro", err)
	}

	return resultado.decisao
}

func registrarNoDiario(ctx context.Context, tentativa Tentativa, causa error) {
	atributos := []any{
		"id_acao", tentativa.IDAcao,
		"tipo", tentativa.Tipo,
		"usuario_slack", tentativa.UsuarioSlack,
		"login_github", tentativa.LoginGitHub,
		"repositorio", tentativa.Repositorio,
		"numero_pr", tentativa.NumeroPR,
		"decisao", tentativa.Decisao,
		"motivo", tentativa.Motivo,
	}
	if causa != nil {
		atributos = append(atributos, "erro", causa.Error())
	}

	slog.Log(ctx, nivelDoDiarioPorDecisao[tentativa.Decisao], "ação remota decidida", atributos...)
}

func exigirDependencias(opcoes OpcoesServico) error {
	dependencias := []struct {
		nome     string
		presente bool
	}{
		{"Identidades", opcoes.Identidades != nil},
		{"Permissoes", opcoes.Permissoes != nil},
		{"Autoria", opcoes.Autoria != nil},
		{"Executor", opcoes.Executor != nil},
		{"Tentativas", opcoes.Tentativas != nil},
		{"Reservas", opcoes.Reservas != nil},
	}

	for _, dependencia := range dependencias {
		if !dependencia.presente {
			return fmt.Errorf("%w: %s", ErrDependenciaAusente, dependencia.nome)
		}
	}
	return nil
}

func (d desfecho) com(decisao Decisao, motivo Motivo, causa error) desfecho {
	d.decisao = decisao
	d.motivo = motivo
	d.causa = causa
	return d
}
