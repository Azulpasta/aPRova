package acaoremota

import (
	"context"
	"time"

	"github.com/Azulpasta/aPRova/internal/platform"
)

// Tipo identifica a ação que o canal externo pede para executar.
type Tipo string

// Ações aceitas por canal externo.
const (
	TipoAprovar            Tipo = "aprovar"
	TipoSolicitarAlteracao Tipo = "solicitar_alteracao"
	TipoReexecutarAnalise  Tipo = "reexecutar_analise"
)

// Decisao é o desfecho de um pedido de ação remota.
type Decisao string

// Desfechos possíveis de um pedido de ação remota.
const (
	DecisaoAceita   Decisao = "aceita"
	DecisaoRecusada Decisao = "recusada"
	DecisaoInvalida Decisao = "invalida"
	DecisaoRepetida Decisao = "repetida"
	DecisaoFalha    Decisao = "falha"
)

// Motivo detalha a decisão no registro. Nunca é devolvido a quem pediu.
type Motivo string

// Motivos registrados junto de cada decisão.
const (
	MotivoAutorizada            Motivo = "autorizada"
	MotivoCorpoInvalido         Motivo = "corpo_invalido"
	MotivoSemVinculo            Motivo = "sem_vinculo"
	MotivoFalhaNaIdentidade     Motivo = "falha_na_identidade"
	MotivoFalhaNaVerificacao    Motivo = "falha_na_verificacao"
	MotivoPermissaoInsuficiente Motivo = "permissao_insuficiente"
	MotivoAutorDoProprioPR      Motivo = "autor_do_proprio_pr"
	MotivoAcaoRepetida          Motivo = "acao_repetida"
	MotivoFalhaNaReserva        Motivo = "falha_na_reserva"
	MotivoFalhaNaExecucao       Motivo = "falha_na_execucao"
)

// Pedido é o corpo enviado pelo canal externo. Diz apenas quem é o usuário no
// canal; o login no GitHub é resolvido pelo núcleo.
type Pedido struct {
	IDAcao         string `json:"id_acao" validate:"required,max=200"`
	Tipo           Tipo   `json:"tipo" validate:"required,oneof=aprovar solicitar_alteracao reexecutar_analise"`
	UsuarioSlack   string `json:"usuario_slack" validate:"required,max=100"`
	Repositorio    string `json:"repositorio" validate:"required,repositorio_github"`
	NumeroPR       int    `json:"numero_pr" validate:"required,gt=0"`
	InstallationID int64  `json:"installation_id" validate:"required,gt=0"`
	Justificativa  string `json:"justificativa" validate:"required_if=Tipo solicitar_alteracao,max=4000"`
}

// AcaoAutorizada é um pedido que passou pela autorização, acompanhado do login
// do humano que decidiu.
type AcaoAutorizada struct {
	Pedido
	LoginGitHub string
}

// Tentativa é o registro de um pedido autenticado, aceito ou recusado.
type Tentativa struct {
	IDAcao       string
	Tipo         Tipo
	UsuarioSlack string
	LoginGitHub  string
	Repositorio  string
	NumeroPR     int
	Decisao      Decisao
	Motivo       Motivo
	Em           time.Time
}

// ResolvedorIdentidade traduz o usuário do canal externo para o login no GitHub.
type ResolvedorIdentidade interface {
	LoginGitHub(ctx context.Context, usuarioSlack string) (string, error)
}

// ConsultorPermissao informa o papel de um login num repositório.
type ConsultorPermissao interface {
	PermissaoNoRepositorio(ctx context.Context, installationID int64, dono, repositorio, login string) (platform.Permissao, error)
}

// ConsultorAutoria informa o login de quem abriu o pull request.
type ConsultorAutoria interface {
	AutorDoPullRequest(ctx context.Context, installationID int64, dono, repositorio string, numero int) (string, error)
}

// ExecutorAcao produz o efeito de uma ação já autorizada.
type ExecutorAcao interface {
	Executar(ctx context.Context, acao AcaoAutorizada) error
}

// ReenfileiradorAnalise devolve um pull request à fila de análise.
type ReenfileiradorAnalise interface {
	Reenfileirar(ctx context.Context, acao AcaoAutorizada) error
}

// RegistroTentativas guarda cada pedido autenticado e o seu desfecho.
type RegistroTentativas interface {
	Registrar(ctx context.Context, tentativa Tentativa) error
}

// ReservaDeAcoes garante que cada id_acao produza efeito uma única vez.
// Reservar devolve false quando o id_acao já foi reservado antes.
type ReservaDeAcoes interface {
	Reservar(ctx context.Context, idAcao string) (bool, error)
}
