package conteiner

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

const prazoMaximo = 24 * time.Hour

var (
	// ErrAmbienteInvalido indica variável APROVA_* ausente ou inválida.
	ErrAmbienteInvalido = errors.New("sandbox: ambiente de execução inválido")

	formatoDoSHA         = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	esquemasAceitos      = []string{"https", "file"}
	linguagensConhecidas = []sandbox.Linguagem{sandbox.LinguagemGo, sandbox.LinguagemPython, sandbox.LinguagemJava}
)

// Ambiente é o pedido recebido pelo executor de dentro do container. Diretorio
// é o caminho do projeto dentro do repositório, vazio para a raiz.
type Ambiente struct {
	Repositorio string
	SHA         string
	Linguagem   sandbox.Linguagem
	TaskRunID   string
	Prazo       time.Duration
	Diretorio   string
}

type leitorDeAmbiente struct {
	ler        func(string) string
	pendencias []string
}

// LerAmbiente lê e valida as variáveis APROVA_*. O erro nomeia cada variável
// ausente ou inválida.
func LerAmbiente(ler func(string) string) (Ambiente, error) {
	leitura := &leitorDeAmbiente{ler: ler}

	ambiente := Ambiente{
		Repositorio: leitura.repositorio("APROVA_REPOSITORIO"),
		SHA:         leitura.sha("APROVA_SHA"),
		Linguagem:   leitura.linguagem("APROVA_LINGUAGEM"),
		TaskRunID:   leitura.obrigatoria("APROVA_TASK_RUN_ID"),
		Prazo:       leitura.prazo("APROVA_PRAZO_SEGUNDOS"),
		Diretorio:   leitura.diretorio("APROVA_DIRETORIO"),
	}

	if len(leitura.pendencias) > 0 {
		return Ambiente{}, fmt.Errorf("%w: %s", ErrAmbienteInvalido, strings.Join(leitura.pendencias, "; "))
	}
	return ambiente, nil
}

func (l *leitorDeAmbiente) obrigatoria(nome string) string {
	valor := strings.TrimSpace(l.ler(nome))
	if valor == "" {
		l.pendenciar("%s não definida", nome)
	}
	return valor
}

func (l *leitorDeAmbiente) repositorio(nome string) string {
	valor := l.obrigatoria(nome)
	if valor == "" {
		return ""
	}

	endereco, err := url.Parse(valor)
	switch {
	case err != nil || !slices.Contains(esquemasAceitos, endereco.Scheme):
		l.pendenciar("%s precisa ser uma URL https ou file", nome)
	case endereco.User != nil:
		l.pendenciar("%s não pode conter credenciais", nome)
	}
	return valor
}

func (l *leitorDeAmbiente) sha(nome string) string {
	valor := strings.ToLower(l.obrigatoria(nome))
	if valor != "" && !formatoDoSHA.MatchString(valor) {
		l.pendenciar("%s precisa ser um SHA completo em hexadecimal, 40 ou 64 caracteres", nome)
	}
	return valor
}

func (l *leitorDeAmbiente) linguagem(nome string) sandbox.Linguagem {
	valor := sandbox.Linguagem(l.obrigatoria(nome))
	if valor != "" && !slices.Contains(linguagensConhecidas, valor) {
		l.pendenciar("%s aceita go, python ou java, obtive %q", nome, valor)
	}
	return valor
}

func (l *leitorDeAmbiente) prazo(nome string) time.Duration {
	valor := l.obrigatoria(nome)
	if valor == "" {
		return 0
	}

	segundos, err := strconv.Atoi(valor)
	prazo := time.Duration(segundos) * time.Second
	if err != nil || prazo <= 0 || prazo > prazoMaximo {
		l.pendenciar("%s precisa ser um número inteiro de segundos entre 1 e %d", nome, int(prazoMaximo.Seconds()))
		return 0
	}
	return prazo
}

func (l *leitorDeAmbiente) diretorio(nome string) string {
	valor := l.ler(nome)
	if err := sandbox.ValidarDiretorioDoProjeto(valor); err != nil {
		l.pendenciar("%s: %v", nome, err)
	}
	return valor
}

func (l *leitorDeAmbiente) pendenciar(formato string, argumentos ...any) {
	l.pendencias = append(l.pendencias, fmt.Sprintf(formato, argumentos...))
}
