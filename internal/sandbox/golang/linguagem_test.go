package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

var agoraDeTeste = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type rodadorFalso struct {
	respostas map[string]processo.Execucao
	rodados   []processo.Comando
	agora     time.Time
	custos    map[string]time.Duration
}

func (r *rodadorFalso) Rodar(_ context.Context, comando processo.Comando) processo.Execucao {
	r.rodados = append(r.rodados, comando)
	r.agora = r.agora.Add(r.custos[subcomando(comando)])
	return r.respostas[subcomando(comando)]
}

func (r *rodadorFalso) relogio() time.Time {
	return r.agora
}

func subcomando(comando processo.Comando) string {
	return strings.Join(comando.Argumentos[:min(2, len(comando.Argumentos))], " ")
}

func (r *rodadorFalso) subcomandosRodados() []string {
	var nomes []string
	for _, comando := range r.rodados {
		nomes = append(nomes, subcomando(comando))
	}
	return nomes
}

func repositorioComGoMod(t *testing.T) string {
	t.Helper()

	diretorio := t.TempDir()
	if err := os.WriteFile(filepath.Join(diretorio, "go.mod"), []byte("module exemplo.local/x\n"), 0o600); err != nil {
		t.Fatalf("escrever go.mod: %v", err)
	}
	return diretorio
}

func executarComRespostas(t *testing.T, diretorio string, respostas map[string]processo.Execucao) (sandbox.ResultadoSandbox, *rodadorFalso) {
	t.Helper()

	rodador := &rodadorFalso{respostas: respostas, agora: agoraDeTeste}
	linguagem := Nova(Opcoes{
		Rodador:          rodador,
		DiretorioDeCache: "/tmp/aprova-cache",
		Agora:            rodador.relogio,
	})

	resultado := linguagem.Executar(t.Context(), diretorio, agoraDeTeste.Add(10*time.Minute))
	return resultado, rodador
}

func testeComGravacao(t *testing.T, gravacao string, codigo int) processo.Execucao {
	return processo.Execucao{Saida: lerGravacao(t, gravacao), CodigoSaida: codigo}
}

func TestExecutarSemGoModNaRaizEhInfraestrutura(t *testing.T) {
	resultado, rodador := executarComRespostas(t, t.TempDir(), nil)

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("sem go.mod a culpa não é do PR: esperava infraestrutura, obtive %q", resultado.TipoFalha)
	}
	if !strings.Contains(resultado.SaidaBruta, "repositório sem go.mod na raiz") {
		t.Errorf("a saída precisa explicar o motivo: %q", resultado.SaidaBruta)
	}
	if len(rodador.rodados) != 0 {
		t.Errorf("nenhum comando go deveria rodar, rodaram %v", rodador.subcomandosRodados())
	}
}

func TestExecutarComTodosOsTestesPassando(t *testing.T) {
	resultado, rodador := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": testeComGravacao(t, "so_passando.jsonl", 0),
	})

	esperado := sandbox.ResultadoSandbox{Compilou: true, TestesPassaram: 3, TipoFalha: sandbox.FalhaNenhuma}
	resultado.SaidaBruta = ""
	if resultado != esperado {
		t.Errorf("obtive %+v, esperava %+v", resultado, esperado)
	}
	if ordem := rodador.subcomandosRodados(); !slices.Equal(ordem, []string{"mod download", "build ./...", "test -c", "test -json"}) {
		t.Errorf("ordem dos comandos: %v", ordem)
	}
}

func TestExecutarComTesteFalhoEhFalhaDeTeste(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": testeComGravacao(t, "um_falho.jsonl", 1),
	})

	if resultado.TipoFalha != sandbox.FalhaTeste || !resultado.Compilou ||
		resultado.TestesPassaram != 2 || resultado.TestesFalharam != 1 {
		t.Errorf("obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "frete: esperava 16, obtive 15") {
		t.Errorf("a saída do teste precisa chegar ao resultado: %q", resultado.SaidaBruta)
	}
}

func TestExecutarComTesteQueTravaEhFalhaDeTeste(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": testeComGravacao(t, "teste_que_trava.jsonl", 1),
	})

	if resultado.TipoFalha != sandbox.FalhaTeste || resultado.TestesFalharam < 1 {
		t.Errorf("travar até o -timeout é culpa do PR, obtive %+v", resultado)
	}
}

func TestExecutarComArquivoDeTesteQueNaoCompilaEhCompilacao(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": testeComGravacao(t, "teste_nao_compila.jsonl", 1),
	})

	if resultado.TipoFalha != sandbox.FalhaCompilacao || resultado.Compilou {
		t.Errorf("teste que não compila é falha de compilação, obtive %+v", resultado)
	}
}

func TestExecutarComBuildQuebradoNaoRodaOsTestes(t *testing.T) {
	resultado, rodador := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"build ./...": {Saida: []byte("./frete.go:3:1: syntax error\n"), CodigoSaida: 1},
	})

	if resultado.TipoFalha != sandbox.FalhaCompilacao || resultado.Compilou {
		t.Errorf("build quebrado é compilação, obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "syntax error") {
		t.Errorf("a saída do build precisa chegar ao resultado: %q", resultado.SaidaBruta)
	}
	if slices.Contains(rodador.subcomandosRodados(), "test -json") {
		t.Error("com o build quebrado os testes não rodam")
	}
}

func TestExecutarComDownloadQuebradoEhInfraestrutura(t *testing.T) {
	resultado, rodador := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"mod download": {Saida: []byte("dial tcp: lookup proxy.golang.org: no such host\n"), CodigoSaida: 1},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("falha de rede no download não é culpa do PR, obtive %q", resultado.TipoFalha)
	}
	if len(rodador.rodados) != 1 {
		t.Errorf("nada roda depois do download quebrado, rodaram %v", rodador.subcomandosRodados())
	}
	if !strings.Contains(resultado.SaidaBruta, "no such host") || !strings.Contains(resultado.SaidaBruta, "go mod download") {
		t.Errorf("a saída precisa dizer qual passo falhou e por quê: %q", resultado.SaidaBruta)
	}
}

func TestExecutarComGoModExigindoGoMaisNovoExplicaOMotivo(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"mod download": {
			Saida:       []byte("go: go.mod requires go >= 1.99 (running go 1.27.1; GOTOOLCHAIN=local)\n"),
			CodigoSaida: 1,
		},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("esperava infraestrutura, obtive %q", resultado.TipoFalha)
	}
	if !strings.Contains(resultado.SaidaBruta, "go.mod exige uma versão do Go mais nova que a da imagem") {
		t.Errorf("a mensagem precisa ser clara: %q", resultado.SaidaBruta)
	}
}

func TestExecutarQuandoOGoNemIniciaEhInfraestrutura(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"mod download": {Erro: errors.New("exec: \"go\": executable file not found in $PATH")},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "executable file not found") {
		t.Errorf("obtive %+v", resultado)
	}
}

func TestExecutarQuandoOPrazoDoExecutorEstouraNoTesteEhInfraestrutura(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": {PrazoEsgotado: true},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("prazo do executor estourado sem resposta do go test é infraestrutura, obtive %q", resultado.TipoFalha)
	}
}

func TestExecutarComGoTestFalhandoSemRelatarNadaEhInfraestrutura(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -json": {Saida: []byte("go: error obtaining buildID\n"), CodigoSaida: 1},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("sem evidência de teste falho não dá para culpar o PR, obtive %q", resultado.TipoFalha)
	}
}

func TestExecutarRodaOsComandosNoRepositorioComAmbienteRestrito(t *testing.T) {
	diretorio := repositorioComGoMod(t)
	_, rodador := executarComRespostas(t, diretorio, map[string]processo.Execucao{
		"test -json": testeComGravacao(t, "so_passando.jsonl", 0),
	})

	if len(rodador.rodados) != 4 {
		t.Fatalf("esperava download, build, compilação dos testes e teste, rodaram %v", rodador.subcomandosRodados())
	}
	for _, comando := range rodador.rodados {
		if comando.Binario != "go" || comando.Diretorio != diretorio {
			t.Errorf("comando %v rodou fora do repositório: %+v", comando.Argumentos, comando)
		}
		for _, exigida := range []string{"GOFLAGS=-mod=readonly", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOCACHE=/tmp/aprova-cache/build"} {
			if !slices.Contains(comando.Ambiente, exigida) {
				t.Errorf("comando %v sem %s no ambiente: %v", comando.Argumentos, exigida, comando.Ambiente)
			}
		}
	}
}

func TestExecutarCompilaOsTestesAntesDeCalcularOTimeout(t *testing.T) {
	rodador := &rodadorFalso{
		respostas: map[string]processo.Execucao{"test -json": testeComGravacao(t, "so_passando.jsonl", 0)},
		agora:     agoraDeTeste,
		custos:    map[string]time.Duration{"build ./...": 20 * time.Second, "test -c": 40 * time.Second},
	}
	linguagem := Nova(Opcoes{Rodador: rodador, DiretorioDeCache: "/tmp/aprova-cache", Agora: rodador.relogio})

	linguagem.Executar(t.Context(), repositorioComGoMod(t), agoraDeTeste.Add(10*time.Minute))

	compilacao := rodador.rodados[2]
	if !slices.Equal(compilacao.Argumentos, []string{"test", "-c", "-o", "/tmp/aprova-cache/binarios-de-teste/", "./..."}) {
		t.Errorf("compilação dos testes = %v", compilacao.Argumentos)
	}
	teste := rodador.rodados[len(rodador.rodados)-1]
	esperados := []string{"test", "-json", "-timeout=510s", "./..."}
	if !slices.Equal(teste.Argumentos, esperados) {
		t.Errorf("argumentos = %v, esperava %v", teste.Argumentos, esperados)
	}
}

func TestExecutarSemTempoParaOsTestesEhInfraestrutura(t *testing.T) {
	rodador := &rodadorFalso{}
	linguagem := Nova(Opcoes{
		Rodador:          rodador,
		DiretorioDeCache: "/tmp/aprova-cache",
		Agora:            func() time.Time { return agoraDeTeste },
	})

	resultado := linguagem.Executar(t.Context(), repositorioComGoMod(t), agoraDeTeste.Add(20*time.Second))

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("sem tempo para testar não há como culpar o PR, obtive %q", resultado.TipoFalha)
	}
	if slices.Contains(rodador.subcomandosRodados(), "test -json") {
		t.Error("go test não pode rodar com timeout que não cabe no prazo")
	}
}

func TestExecutarComTesteQueNaoCompilaNaoRodaOsTestes(t *testing.T) {
	resultado, rodador := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -c": {Saida: []byte("./frete_test.go:5:43: cannot use \"texto\" as int value\n"), CodigoSaida: 1},
	})

	if resultado.TipoFalha != sandbox.FalhaCompilacao || resultado.Compilou {
		t.Errorf("teste que não compila é compilação, obtive %+v", resultado)
	}
	if !strings.Contains(resultado.SaidaBruta, "cannot use") {
		t.Errorf("o erro de compilação precisa chegar ao resultado: %q", resultado.SaidaBruta)
	}
	if slices.Contains(rodador.subcomandosRodados(), "test -json") {
		t.Error("com teste que não compila, go test não roda")
	}
}

func TestExecutarComCompilacaoDosTestesInterrompidaEhInfraestrutura(t *testing.T) {
	resultado, _ := executarComRespostas(t, repositorioComGoMod(t), map[string]processo.Execucao{
		"test -c": {PrazoEsgotado: true},
	})

	if resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("prazo estourado compilando é infraestrutura, obtive %q", resultado.TipoFalha)
	}
}
