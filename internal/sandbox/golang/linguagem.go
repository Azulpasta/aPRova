package golang

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

const (
	binarioGo = "go"

	margemParaEntregar    = 30 * time.Second
	tempoMinimoDosTestes  = 30 * time.Second
	sinalDeGoMaisNovo     = "requires go >="
	caminhoDosExecutaveis = "PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
)

// Opcoes configura a execução de build e testes de um repositório Go.
// DiretorioDeCache recebe o cache de build e o de módulos.
type Opcoes struct {
	Rodador          processo.Rodador
	DiretorioDeCache string
	Agora            func() time.Time
}

// Linguagem roda build e testes de repositórios Go.
type Linguagem struct {
	rodador          processo.Rodador
	diretorioDeCache string
	agora            func() time.Time
}

// Nova constrói a linguagem Go com as opções informadas. Sem relógio, usa
// time.Now.
func Nova(opcoes Opcoes) *Linguagem {
	agora := opcoes.Agora
	if agora == nil {
		agora = time.Now
	}
	return &Linguagem{rodador: opcoes.Rodador, diretorioDeCache: opcoes.DiretorioDeCache, agora: agora}
}

// Executar roda download de módulos, build e testes no diretório informado e
// classifica o desfecho. prazoFinal é o instante em que o resultado já precisa
// ter sido entregue.
func (l *Linguagem) Executar(ctx context.Context, diretorio string, prazoFinal time.Time) sandbox.ResultadoSandbox {
	if !existeGoModNaRaiz(diretorio) {
		return infraestrutura("repositório sem go.mod na raiz")
	}

	download := l.rodar(ctx, diretorio, "mod", "download")
	if !concluiu(download) {
		return infraestrutura(explicarFalhaDoDownload(download), download.Saida)
	}

	build := l.rodar(ctx, diretorio, "build", "./...")
	if !concluiu(build) {
		return classificarCompilacao("go build", build)
	}

	compilacaoDosTestes := l.rodar(ctx, diretorio, "test", "-c", "-o", l.diretorioDosBinariosDeTeste(), "./...")
	if !concluiu(compilacaoDosTestes) {
		return classificarCompilacao("compilação dos testes", compilacaoDosTestes)
	}

	timeout := prazoFinal.Sub(l.agora()) - margemParaEntregar
	if timeout < tempoMinimoDosTestes {
		return infraestrutura(fmt.Sprintf("prazo insuficiente para rodar os testes: restam %v", timeout))
	}

	teste := l.rodar(ctx, diretorio, "test", "-json", fmt.Sprintf("-timeout=%ds", int(timeout.Seconds())), "./...")
	return classificarTeste(teste)
}

func (l *Linguagem) rodar(ctx context.Context, diretorio string, argumentos ...string) processo.Execucao {
	return l.rodador.Rodar(ctx, processo.Comando{
		Binario:    binarioGo,
		Argumentos: argumentos,
		Diretorio:  diretorio,
		Ambiente:   l.ambiente(),
	})
}

func (l *Linguagem) ambiente() []string {
	return []string{
		caminhoDosExecutaveis,
		"HOME=" + filepath.Join(l.diretorioDeCache, "home"),
		"GOPATH=" + filepath.Join(l.diretorioDeCache, "gopath"),
		"GOCACHE=" + filepath.Join(l.diretorioDeCache, "build"),
		"GOFLAGS=-mod=readonly",
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
	}
}

func (l *Linguagem) diretorioDosBinariosDeTeste() string {
	return filepath.Join(l.diretorioDeCache, "binarios-de-teste") + string(filepath.Separator)
}

func classificarCompilacao(etapa string, compilacao processo.Execucao) sandbox.ResultadoSandbox {
	if compilacao.Erro != nil || compilacao.PrazoEsgotado {
		return infraestrutura(descreverInterrupcao(etapa, compilacao), compilacao.Saida)
	}
	return sandbox.ResultadoSandbox{Compilou: false, SaidaBruta: string(compilacao.Saida), TipoFalha: sandbox.FalhaCompilacao}
}

func classificarTeste(teste processo.Execucao) sandbox.ResultadoSandbox {
	if teste.Erro != nil || teste.PrazoEsgotado {
		return infraestrutura(descreverInterrupcao("go test", teste), teste.Saida)
	}

	apurada := apurar(teste.Saida)
	resultado := sandbox.ResultadoSandbox{
		Compilou:       !apurada.falhaDeCompilacao,
		TestesPassaram: apurada.passaram,
		TestesFalharam: apurada.falharam,
		SaidaBruta:     apurada.saida,
	}

	switch {
	case apurada.falhaDeCompilacao:
		resultado.TipoFalha = sandbox.FalhaCompilacao
	case apurada.falharam > 0:
		resultado.TipoFalha = sandbox.FalhaTeste
	case teste.CodigoSaida != 0:
		return infraestrutura(fmt.Sprintf("go test saiu com código %d sem relatar teste falho", teste.CodigoSaida), teste.Saida)
	default:
		resultado.TipoFalha = sandbox.FalhaNenhuma
	}

	return resultado
}

func concluiu(execucao processo.Execucao) bool {
	return execucao.Erro == nil && !execucao.PrazoEsgotado && execucao.CodigoSaida == 0
}

func explicarFalhaDoDownload(download processo.Execucao) string {
	if strings.Contains(string(download.Saida), sinalDeGoMaisNovo) {
		return "o go.mod exige uma versão do Go mais nova que a da imagem do sandbox"
	}
	return descreverInterrupcao("go mod download", download)
}

func descreverInterrupcao(etapa string, execucao processo.Execucao) string {
	switch {
	case execucao.Erro != nil:
		return fmt.Sprintf("%s não pôde ser iniciado: %v", etapa, execucao.Erro)
	case execucao.PrazoEsgotado:
		return fmt.Sprintf("%s excedeu o prazo do executor", etapa)
	default:
		return fmt.Sprintf("%s falhou com código %d", etapa, execucao.CodigoSaida)
	}
}

func existeGoModNaRaiz(diretorio string) bool {
	informacao, err := os.Stat(filepath.Join(diretorio, "go.mod"))
	return err == nil && informacao.Mode().IsRegular()
}

func infraestrutura(motivo string, saidas ...[]byte) sandbox.ResultadoSandbox {
	var saida strings.Builder
	for _, trecho := range saidas {
		saida.Write(trecho)
	}
	saida.WriteString("aprova: " + motivo + "\n")

	return sandbox.ResultadoSandbox{SaidaBruta: saida.String(), TipoFalha: sandbox.FalhaInfraestrutura}
}
