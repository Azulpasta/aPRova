package integracao

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

const (
	imagemDoSandboxGo     = "aprova-sandbox-go:teste-de-integracao"
	tempoLimiteDoBuild    = 10 * time.Minute
	pontoDeMontagem       = "/repositorios"
	prazoDeUmCasoComum    = 3 * time.Minute
	prazoDoCasoQueTrava   = 2 * time.Minute
	prefixoDosRepositorio = ".repositorios-"
)

var (
	construcaoDaImagem sync.Once
	erroDaConstrucao   error
	saidaDaConstrucao  []byte
)

func exigirImagemDoSandboxGo(t *testing.T) {
	t.Helper()
	exigirDocker(t)

	construcaoDaImagem.Do(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteDoBuild)
		defer cancelar()

		raiz, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			erroDaConstrucao = err
			return
		}
		construcao := exec.CommandContext(ctx, "docker", "build", "-q",
			"-f", filepath.Join(raiz, "imagens", "go", "Dockerfile"), "-t", imagemDoSandboxGo, raiz)
		saidaDaConstrucao, erroDaConstrucao = construcao.CombinedOutput()
	})

	if erroDaConstrucao != nil {
		t.Fatalf("construir a imagem do sandbox Go: %v\n%s", erroDaConstrucao, saidaDaConstrucao)
	}
}

type repositoriosLocais struct {
	diretorio string
}

func novosRepositoriosLocais(t *testing.T) repositoriosLocais {
	t.Helper()

	relativo, err := os.MkdirTemp(".", prefixoDosRepositorio)
	if err != nil {
		t.Fatalf("criar diretório dos repositórios: %v", err)
	}
	diretorio, err := filepath.Abs(relativo)
	if err != nil {
		t.Fatalf("caminho absoluto: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(diretorio) })

	if err := os.Chmod(diretorio, 0o755); err != nil {
		t.Fatalf("o uid 10001 do container precisa atravessar o diretório montado: %v", err)
	}

	return repositoriosLocais{diretorio: diretorio}
}

func (r repositoriosLocais) publicar(t *testing.T, caso string, ajustar func(t *testing.T, destino string)) (endereco, sha string) {
	t.Helper()

	destino := filepath.Join(r.diretorio, caso)
	copiarDiretorio(t, filepath.Join("testdata", "go", caso), destino)
	if ajustar != nil {
		ajustar(t, destino)
	}

	gitLocal(t, destino, "init", "-q")
	gitLocal(t, destino, "add", ".")
	gitLocal(t, destino, "commit", "-q", "-m", "caso "+caso)
	sha = gitLocal(t, destino, "rev-parse", "HEAD")

	return "file://" + pontoDeMontagem + "/" + caso, sha
}

func copiarDiretorio(t *testing.T, origem, destino string) {
	t.Helper()

	err := filepath.WalkDir(origem, func(caminho string, entrada fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativo, err := filepath.Rel(origem, caminho)
		if err != nil {
			return err
		}
		alvo := filepath.Join(destino, relativo)
		if entrada.IsDir() {
			return os.MkdirAll(alvo, 0o755)
		}
		conteudo, err := os.ReadFile(caminho)
		if err != nil {
			return err
		}
		return os.WriteFile(alvo, conteudo, 0o644)
	})
	if err != nil {
		t.Fatalf("copiar %s: %v", origem, err)
	}
}

func gitLocal(t *testing.T, diretorio string, argumentos ...string) string {
	t.Helper()

	comando := exec.CommandContext(t.Context(), "git", append([]string{"-C", diretorio}, argumentos...)...)
	comando.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=aprova", "GIT_AUTHOR_EMAIL=aprova@exemplo.local",
		"GIT_COMMITTER_NAME=aprova", "GIT_COMMITTER_EMAIL=aprova@exemplo.local")
	saida, err := comando.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", argumentos, err, saida)
	}
	return strings.TrimSpace(string(saida))
}

type cenarioDeIntegracao struct {
	caso      string
	prazo     time.Duration
	opcoes    sandbox.OpcoesExecutorLocal
	diretorio string
	ajustar   func(t *testing.T, destino string)
}

func executarCaso(t *testing.T, caso string, prazo time.Duration) sandbox.ResultadoSandbox {
	t.Helper()
	return executarCenario(t, cenarioDeIntegracao{caso: caso, prazo: prazo})
}

func executarCasoComOpcoes(t *testing.T, caso string, prazo time.Duration, opcoes sandbox.OpcoesExecutorLocal) sandbox.ResultadoSandbox {
	t.Helper()
	return executarCenario(t, cenarioDeIntegracao{caso: caso, prazo: prazo, opcoes: opcoes})
}

func executarCenario(t *testing.T, cenario cenarioDeIntegracao) sandbox.ResultadoSandbox {
	t.Helper()
	exigirImagemDoSandboxGo(t)

	caso, prazo, opcoes := cenario.caso, cenario.prazo, cenario.opcoes
	repositorios := novosRepositoriosLocais(t)
	endereco, sha := repositorios.publicar(t, caso, cenario.ajustar)

	opcoes.ImagemPorLinguagem = map[sandbox.Linguagem]string{sandbox.LinguagemGo: imagemDoSandboxGo}
	opcoes.Montagens = []string{repositorios.diretorio + ":" + pontoDeMontagem + ":ro"}
	executor := sandbox.NovoExecutorLocal(opcoes)

	ctx, cancelar := context.WithTimeout(t.Context(), prazo)
	defer cancelar()

	resultado, err := executor.Executar(ctx, sandbox.PedidoExecucao{
		URLRepositorio: endereco,
		SHA:            sha,
		Linguagem:      sandbox.LinguagemGo,
		TaskRunID:      "integracao-" + caso,
		Diretorio:      cenario.diretorio,
	})
	if err != nil {
		t.Fatalf("executar caso %s: %v", caso, err)
	}

	t.Logf("caso %s: tipo=%s compilou=%v passaram=%d falharam=%d duracao=%dms\n%s",
		caso, resultado.TipoFalha, resultado.Compilou, resultado.TestesPassaram, resultado.TestesFalharam,
		resultado.DuracaoMS, resultado.SaidaBruta)

	return resultado
}

func TestSandboxGoProjetoOk(t *testing.T) {
	resultado := executarCaso(t, "ok", prazoDeUmCasoComum)

	if resultado.TipoFalha != sandbox.FalhaNenhuma || !resultado.Compilou ||
		resultado.TestesPassaram != 2 || resultado.TestesFalharam != 0 {
		t.Errorf("projeto ok: esperava 2 testes passando sem falha, obtive %+v", resultado)
	}
}

func TestSandboxGoTesteFalho(t *testing.T) {
	resultado := executarCaso(t, "teste_falho", prazoDeUmCasoComum)

	if resultado.TipoFalha != sandbox.FalhaTeste || !resultado.Compilou ||
		resultado.TestesPassaram != 2 || resultado.TestesFalharam != 1 {
		t.Errorf("esperava falha de teste com 2 passando e 1 falhando, obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "frete com taxa: esperava 16, obtive 15") {
		t.Error("a mensagem do teste falho precisa estar na saída")
	}
}

func TestSandboxGoErroDeCompilacao(t *testing.T) {
	resultado := executarCaso(t, "nao_compila", prazoDeUmCasoComum)

	if resultado.TipoFalha != sandbox.FalhaCompilacao || resultado.Compilou ||
		resultado.TestesPassaram != 0 || resultado.TestesFalharam != 0 {
		t.Errorf("esperava compilação sem nenhum teste rodado, obtive %+v", resultado)
	}
}

func TestSandboxGoTesteQueTravaEhFalhaDeTesteDentroDoPrazo(t *testing.T) {
	inicio := time.Now()

	resultado := executarCaso(t, "trava", prazoDoCasoQueTrava)

	if resultado.TipoFalha != sandbox.FalhaTeste || resultado.TestesFalharam < 1 {
		t.Errorf("teste que trava é culpa do PR, obtive %+v", resultado)
	}
	if decorrido := time.Since(inicio); decorrido >= prazoDoCasoQueTrava {
		t.Errorf("o resultado precisa chegar dentro do prazo, levou %v", decorrido)
	}
}

func TestSandboxGoTesteMaliciosoNaoForjaOResultado(t *testing.T) {
	resultado := executarCaso(t, "malicioso", prazoDeUmCasoComum)

	if resultado.TestesPassaram == 99 || resultado.SaidaBruta == "forjado" {
		t.Fatalf("o resultado forjado venceu: %+v", resultado)
	}
	if resultado.TipoFalha != sandbox.FalhaTeste || resultado.TestesFalharam != 1 || resultado.TestesPassaram != 0 {
		t.Errorf("o resultado entregue precisa ser o verdadeiro, a falha; obtive %+v", resultado)
	}
}

func TestSandboxGoModExigindoGoMaisNovoEhInfraestrutura(t *testing.T) {
	resultado := executarCaso(t, "go_mais_novo", prazoDeUmCasoComum)

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("esperava infraestrutura, obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "go.mod exige uma versão do Go mais nova") {
		t.Errorf("a mensagem precisa explicar a versão: %q", resultado.SaidaBruta)
	}
}

func TestSandboxGoSemTrocaDeUsuarioFalhaFechadoEmInfraestrutura(t *testing.T) {
	resultado := executarCasoComOpcoes(t, "ok", prazoDeUmCasoComum, sandbox.OpcoesExecutorLocal{SemCapacidadeDeTrocarDeUsuario: true})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || resultado.Compilou || resultado.TestesPassaram != 0 {
		t.Fatalf("sem troca de usuário o PR não pode rodar como root, obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "não foi possível rodar os testes como usuário sem privilégio") {
		t.Errorf("a mensagem precisa nomear a troca negada: %q", resultado.SaidaBruta)
	}
}

func TestSandboxGoComProjetoNumSubdiretorio(t *testing.T) {
	resultado := executarCenario(t, cenarioDeIntegracao{caso: "subdiretorio", prazo: prazoDeUmCasoComum, diretorio: "go"})

	if resultado.TipoFalha != sandbox.FalhaNenhuma || !resultado.Compilou || resultado.TestesPassaram != 2 {
		t.Errorf("com o diretório do projeto informado, os testes de go/ rodam: %+v", resultado)
	}
}

func TestSandboxGoSemDiretorioNumRepositorioComOProjetoEmSubdiretorio(t *testing.T) {
	resultado := executarCenario(t, cenarioDeIntegracao{caso: "subdiretorio", prazo: prazoDeUmCasoComum})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "sem go.mod") {
		t.Errorf("sem o diretório do projeto não há go.mod na raiz: %+v", resultado)
	}
}

func TestSandboxGoRecusaDiretorioQueOPRTrocouPorLinkParaForaDoClone(t *testing.T) {
	trocarPorLink := func(t *testing.T, destino string) {
		t.Helper()
		if err := os.RemoveAll(filepath.Join(destino, "go")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/usr/local/go", filepath.Join(destino, "go")); err != nil {
			t.Fatal(err)
		}
	}

	resultado := executarCenario(t, cenarioDeIntegracao{
		caso: "subdiretorio", prazo: prazoDeUmCasoComum, diretorio: "go", ajustar: trocarPorLink,
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "fora do repositório") {
		t.Errorf("o link commitado pelo PR não pode levar os testes para fora do clone: %+v", resultado)
	}
}
