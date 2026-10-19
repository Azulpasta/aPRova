package domain

import (
	"context"
	"errors"
	"time"
)

// Erros devolvidos pelos repositórios, independentes do banco por trás.
var (
	ErrNaoEncontrado        = errors.New("domain: registro não encontrado")
	ErrDuplicado            = errors.New("domain: registro já existe")
	ErrDadosInvalidos       = errors.New("domain: dados inválidos")
	ErrTransicaoConcorrente = errors.New("domain: task_run não está mais no estado esperado")
)

// DimensoesEmbedding é o tamanho do vetor guardado em code_memory.
const DimensoesEmbedding = 1536

// EstadoTaskRun é um estado da máquina de estados em docs/maquina-estados.mmd.
type EstadoTaskRun string

// Estados possíveis de um task_run.
const (
	EstadoPendente            EstadoTaskRun = "pendente"
	EstadoMapeandoContexto    EstadoTaskRun = "mapeando_contexto"
	EstadoGerando             EstadoTaskRun = "gerando"
	EstadoTestandoSandbox     EstadoTaskRun = "testando_sandbox"
	EstadoAvaliandoRisco      EstadoTaskRun = "avaliando_risco"
	EstadoAguardandoAprovacao EstadoTaskRun = "aguardando_aprovacao"
	EstadoAplicando           EstadoTaskRun = "aplicando"
	EstadoConcluido           EstadoTaskRun = "concluido"
	EstadoRejeitado           EstadoTaskRun = "rejeitado"
	EstadoFalhou              EstadoTaskRun = "falhou"
)

// NivelRisco é a classificação do gate de risco.
type NivelRisco string

// Níveis de risco.
const (
	RiscoBaixo NivelRisco = "baixo"
	RiscoMedio NivelRisco = "medio"
	RiscoAlto  NivelRisco = "alto"
)

// DecisaoAprovacao é o desfecho de uma revisão humana.
type DecisaoAprovacao string

// Decisões de revisão registradas em approvals.
const (
	DecisaoAprovada              DecisaoAprovacao = "aprovada"
	DecisaoAlteracoesSolicitadas DecisaoAprovacao = "alteracoes_solicitadas"
)

// MotivoFeedback é o motivo declarado por quem aprovou uma mudança de risco alto.
type MotivoFeedback string

// Motivos aceitos no feedback da aprovação.
const (
	FeedbackRegraConservadora MotivoFeedback = "regra_conservadora"
	FeedbackRiscoAceito       MotivoFeedback = "risco_aceito"
	FeedbackFalsoPositivo     MotivoFeedback = "falso_positivo"
)

// DesfechoMudanca é o que aconteceu com uma mudança depois da decisão, usado
// pela calibração do gate.
type DesfechoMudanca string

// Desfechos guardados em code_memory, do sinal mais forte ao mais fraco.
const (
	DesfechoRevertidaAposMerge DesfechoMudanca = "revertida_apos_merge"
	DesfechoAltoRiscoAprovada  DesfechoMudanca = "alto_risco_aprovada"
	DesfechoFechadaSemMerge    DesfechoMudanca = "fechada_sem_merge"
	DesfechoAjustadaEAprovada  DesfechoMudanca = "ajustada_e_aprovada"
	DesfechoSemReversao        DesfechoMudanca = "sem_reversao"
)

// NovoTaskRun descreve a execução criada a partir de um webhook de pull request.
type NovoTaskRun struct {
	TenantID    int64     `validate:"required,gt=0"`
	DeliveryID  string    `validate:"required,max=100"`
	Repositorio string    `validate:"required,max=200"`
	NumeroPR    int       `validate:"required,gt=0"`
	SHAHead     string    `validate:"required,hexadecimal,min=40,max=64"`
	Autor       string    `validate:"required,max=100"`
	Acao        string    `validate:"required,max=50"`
	RecebidoEm  time.Time `validate:"required"`
}

// TaskRun é a raiz de uma análise: tudo o que o aPRova grava pertence a uma.
type TaskRun struct {
	ID           string
	TenantID     int64
	DeliveryID   string
	Repositorio  string
	NumeroPR     int
	SHAHead      string
	Autor        string
	Acao         string
	Estado       EstadoTaskRun
	RecebidoEm   time.Time
	CriadoEm     time.Time
	AtualizadoEm time.Time
}

// Transicao move um task_run de um estado para outro, deixando uma etapa
// registrada em task_steps.
type Transicao struct {
	TaskRunID string        `validate:"required,uuid"`
	De        EstadoTaskRun `validate:"required,oneof=pendente mapeando_contexto gerando testando_sandbox avaliando_risco aguardando_aprovacao aplicando concluido rejeitado falhou"`
	Para      EstadoTaskRun `validate:"required,nefield=De,oneof=pendente mapeando_contexto gerando testando_sandbox avaliando_risco aguardando_aprovacao aplicando concluido rejeitado falhou"`
	Detalhe   string        `validate:"max=4000"`
}

// GateDeRisco é a decisão do gate para um task_run, com o motivo nomeado.
type GateDeRisco struct {
	TaskRunID           string     `validate:"required,uuid"`
	NivelDeterministico NivelRisco `validate:"required,oneof=baixo medio alto"`
	Nivel               NivelRisco `validate:"required,oneof=baixo medio alto"`
	Motivo              string     `validate:"required,max=1000"`
}

// NovoRecibo é o recibo assinado, pronto para ser anexado.
type NovoRecibo struct {
	TaskRunID        string    `validate:"required,uuid"`
	ReciboAnteriorID *string   `validate:"omitempty,uuid"`
	Conteudo         string    `validate:"required"`
	Hash             []byte    `validate:"len=32"`
	Assinatura       []byte    `validate:"len=64"`
	IDChave          string    `validate:"required,max=100"`
	AssinadoEm       time.Time `validate:"required"`
}

// Recibo é o registro imutável da decisão de revisão de um task_run.
type Recibo struct {
	NovoRecibo
	ID       string
	TenantID int64
	CriadoEm time.Time
}

// Aprovacao é a decisão de uma revisão humana sobre um task_run.
type Aprovacao struct {
	TaskRunID      string           `validate:"required,uuid"`
	DeliveryID     string           `validate:"required,max=100"`
	Decisao        DecisaoAprovacao `validate:"required,oneof=aprovada alteracoes_solicitadas"`
	Revisor        string           `validate:"required,max=100"`
	MotivoFeedback *MotivoFeedback  `validate:"omitempty,oneof=regra_conservadora risco_aceito falso_positivo"`
	Justificativa  *string          `validate:"omitempty,max=4000"`
	DecididaEm     time.Time        `validate:"required"`
}

// MemoriaDeCodigo é uma decisão passada guardada para a calibração do gate.
type MemoriaDeCodigo struct {
	TenantID    int64           `validate:"required,gt=0"`
	TaskRunID   *string         `validate:"omitempty,uuid"`
	Repositorio string          `validate:"required,max=200"`
	Resumo      string          `validate:"required"`
	Embedding   []float32       `validate:"len=1536"`
	Nivel       NivelRisco      `validate:"required,oneof=baixo medio alto"`
	Desfecho    DesfechoMudanca `validate:"required,oneof=revertida_apos_merge alto_risco_aprovada fechada_sem_merge ajustada_e_aprovada sem_reversao"`
}

// Vizinho é uma memória de código próxima da consultada, sem o embedding.
type Vizinho struct {
	ID          int64
	TaskRunID   *string
	Repositorio string
	Resumo      string
	Nivel       NivelRisco
	Desfecho    DesfechoMudanca
	Distancia   float64
}

// RepositorioTaskRun cria, consulta e transiciona task_runs.
type RepositorioTaskRun interface {
	CriarTaskRun(ctx context.Context, novo NovoTaskRun) (TaskRun, bool, error)
	BuscarTaskRun(ctx context.Context, id string) (TaskRun, error)
	BuscarTaskRunPorDeliveryID(ctx context.Context, deliveryID string) (TaskRun, error)
	TransicionarTaskRun(ctx context.Context, transicao Transicao) (TaskRun, error)
}

// RepositorioRecibo anexa e lê recibos. Não há como alterar nem apagar um.
type RepositorioRecibo interface {
	AnexarRecibo(ctx context.Context, novo NovoRecibo) (Recibo, error)
	BuscarRecibo(ctx context.Context, id string) (Recibo, error)
	BuscarReciboPorTaskRun(ctx context.Context, taskRunID string) (Recibo, error)
}
