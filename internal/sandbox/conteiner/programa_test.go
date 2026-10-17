package conteiner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
	"github.com/Azulpasta/aPRova/internal/sandbox/processo"
)

var inicioDeTeste = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type linguagemFalsa struct {
	resultado  sandbox.ResultadoSandbox
	diretorio  string
	prazoFinal time.Time
	chamadas   int
}

func (l *linguagemFalsa) Executar(ctx context.Context, diretorio string, prazoFinal time.Time) sandbox.ResultadoSandbox {
	l.chamadas++
	l.diretorio = diretorio
	l.prazoFinal = prazoFinal
	if prazo, definido := ctx.Deadline(); !definido || !prazo.Equal(prazoFinal) {
		l.resultado.SaidaBruta = "contexto sem o prazo do executor"
	}
	return l.resultado
}

type relogioDeTeste struct{ chamadas int }

func (r *relogioDeTeste) agora() time.Time {
	r.chamadas++
	return inicioDeTeste.Add(time.Duration(r.chamadas-1) * 1500 * time.Millisecond)
}

type cenarioDoPrograma struct {
	variaveis map[string]string
	saida     bytes.Buffer
	git       *rodadorDeGitFalso
	linguagem *linguagemFalsa
	coletor   sandbox.ColetorResultado
}

func novoCenarioDoPrograma() *cenarioDoPrograma {
	return &cenarioDoPrograma{
		variaveis: variaveisValidas(),
		git:       &rodadorDeGitFalso{headDevolvido: shaDeTeste},
		linguagem: &linguagemFalsa{resultado: sandbox.ResultadoSandbox{
			Compilou: true, TestesPassaram: 3, SaidaBruta: "ok\n", TipoFalha: sandbox.FalhaNenhuma,
		}},
	}
}

func (c *cenarioDoPrograma) executar(t *testing.T) int {
	t.Helper()

	relogio := &relogioDeTeste{}
	return Programa{
		LerVariavel:         func(nome string) string { return c.variaveis[nome] },
		Saida:               &c.saida,
		Coletor:             c.coletor,
		Rodador:             c.git,
		DiretorioDeTrabalho: "/tmp/aprova",
		Linguagens:          map[sandbox.Linguagem]ExecutorDeLinguagem{sandbox.LinguagemGo: c.linguagem},
		Agora:               relogio.agora,
	}.Executar(t.Context())
}

func (c *cenarioDoPrograma) resultadoPublicado(t *testing.T) sandbox.ResultadoSandbox {
	t.Helper()

	if strings.Count(c.saida.String(), "\n") != 1 {
		t.Fatalf("o stdout precisa ter exatamente uma linha, tem: %q", c.saida.String())
	}
	var resultado sandbox.ResultadoSandbox
	if err := json.Unmarshal(c.saida.Bytes(), &resultado); err != nil {
		t.Fatalf("a linha do stdout não é um resultado: %v", err)
	}
	return resultado
}

func TestProgramaPublicaOResultadoDaLinguagem(t *testing.T) {
	c := novoCenarioDoPrograma()

	codigo := c.executar(t)

	if codigo != 0 {
		t.Errorf("resultado publicado termina com código 0, obtive %d", codigo)
	}
	esperado := sandbox.ResultadoSandbox{
		Compilou: true, TestesPassaram: 3, SaidaBruta: "ok\n", DuracaoMS: 1500, TipoFalha: sandbox.FalhaNenhuma,
	}
	if obtido := c.resultadoPublicado(t); obtido != esperado {
		t.Errorf("obtive %+v, esperava %+v", obtido, esperado)
	}
	if c.linguagem.diretorio != filepath.Join("/tmp/aprova", "repositorio") {
		t.Errorf("a linguagem rodou em %q", c.linguagem.diretorio)
	}
	if !c.linguagem.prazoFinal.Equal(inicioDeTeste.Add(10 * time.Minute)) {
		t.Errorf("prazo final = %v, esperava início + APROVA_PRAZO_SEGUNDOS", c.linguagem.prazoFinal)
	}
}

func TestProgramaPublicaFalhaDeTesteComCodigoZero(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.linguagem.resultado = sandbox.ResultadoSandbox{Compilou: true, TestesFalharam: 1, TipoFalha: sandbox.FalhaTeste}

	if codigo := c.executar(t); codigo != 0 {
		t.Errorf("teste falho é um resultado entregue, não erro do executor: código %d", codigo)
	}
	if c.resultadoPublicado(t).TipoFalha != sandbox.FalhaTeste {
		t.Error("o tipo de falha precisa chegar intacto")
	}
}

func TestProgramaLimitaASaidaBrutaMantendoOFinal(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.linguagem.resultado.SaidaBruta = strings.Repeat("x", 200*1024) + "FAIL TestFrete\n"

	c.executar(t)

	saida := c.resultadoPublicado(t).SaidaBruta
	if len(saida) > 64*1024 || !strings.HasSuffix(saida, "FAIL TestFrete\n") {
		t.Errorf("saída com %d bytes, precisa caber em 64KiB mantendo o final", len(saida))
	}
}

func TestProgramaComAmbienteInvalidoPublicaInfraestrutura(t *testing.T) {
	c := novoCenarioDoPrograma()
	delete(c.variaveis, "APROVA_SHA")
	c.variaveis["APROVA_PRAZO_SEGUNDOS"] = "muito"

	codigo := c.executar(t)

	resultado := c.resultadoPublicado(t)
	if codigo != 0 || resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Errorf("esperava infraestrutura publicada com código 0, obtive %d e %+v", codigo, resultado)
	}
	for _, variavel := range []string{"APROVA_SHA", "APROVA_PRAZO_SEGUNDOS"} {
		if !strings.Contains(resultado.SaidaBruta, variavel) {
			t.Errorf("a mensagem precisa citar %s: %q", variavel, resultado.SaidaBruta)
		}
	}
	if len(c.git.comandos) != 0 || c.linguagem.chamadas != 0 {
		t.Error("com ambiente inválido nada roda")
	}
}

func TestProgramaComLinguagemSemSuportePublicaInfraestrutura(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_LINGUAGEM"] = "python"

	c.executar(t)

	resultado := c.resultadoPublicado(t)
	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "python") {
		t.Errorf("obtive %+v", resultado)
	}
	if len(c.git.comandos) != 0 {
		t.Error("sem linguagem suportada não há por que clonar")
	}
}

func TestProgramaComCloneQuebradoPublicaInfraestrutura(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.git.headDevolvido = strings.Repeat("b", 40)

	c.executar(t)

	resultado := c.resultadoPublicado(t)
	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "diverge") {
		t.Errorf("obtive %+v", resultado)
	}
	if c.linguagem.chamadas != 0 {
		t.Error("sem o SHA certo nada pode ser testado")
	}
}

func TestProgramaComSaidaDesconhecidaPublicaInfraestruturaNoStdout(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_SAIDA"] = "arquivo"

	c.executar(t)

	resultado := c.resultadoPublicado(t)
	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "APROVA_SAIDA") {
		t.Errorf("obtive %+v", resultado)
	}
	if c.linguagem.chamadas != 0 {
		t.Error("sem destino válido para o resultado não vale a pena executar")
	}
}

func TestProgramaComSaidaNoColetorNaoEscreveNoStdout(t *testing.T) {
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_SAIDA"] = "coletor"
	coletor := sandbox.NovoColetorEmMemoria()
	c.coletor = coletor

	if codigo := c.executar(t); codigo != 0 {
		t.Fatalf("código %d", codigo)
	}

	if c.saida.Len() != 0 {
		t.Errorf("com saída no coletor o stdout fica vazio, tem %q", c.saida.String())
	}
	ctx, cancelar := context.WithTimeout(t.Context(), time.Second)
	defer cancelar()
	if gravado, err := coletor.Aguardar(ctx, "task-run-1"); err != nil || gravado.TestesPassaram != 3 {
		t.Errorf("o resultado precisa estar no coletor: %v, %+v", err, gravado)
	}
}

func TestProgramaTerminaComCodigoUmQuandoNaoConsegueEntregar(t *testing.T) {
	c := novoCenarioDoPrograma()
	relogio := &relogioDeTeste{}

	codigo := Programa{
		LerVariavel:         func(nome string) string { return c.variaveis[nome] },
		Saida:               escritorQuebrado{},
		Rodador:             c.git,
		DiretorioDeTrabalho: "/tmp/aprova",
		Linguagens:          map[sandbox.Linguagem]ExecutorDeLinguagem{sandbox.LinguagemGo: c.linguagem},
		Agora:               relogio.agora,
	}.Executar(t.Context())

	if codigo != 1 {
		t.Errorf("sem entregar o resultado o executor precisa falhar, código %d", codigo)
	}
}

type rodadorQueNegaATroca struct {
	tentativas int
}

func (r *rodadorQueNegaATroca) Rodar(context.Context, processo.Comando) processo.Execucao {
	r.tentativas++
	return processo.Execucao{Erro: fmt.Errorf("%w (uid 10001): fork/exec /usr/bin/git: operation not permitted", processo.ErrTrocaDeUsuarioNegada)}
}

func TestProgramaComTrocaDeUsuarioNegadaPublicaInfraestruturaSemRodarNada(t *testing.T) {
	c := novoCenarioDoPrograma()
	negador := &rodadorQueNegaATroca{}
	relogio := &relogioDeTeste{}

	codigo := Programa{
		LerVariavel:         func(nome string) string { return c.variaveis[nome] },
		Saida:               &c.saida,
		Rodador:             negador,
		DiretorioDeTrabalho: "/tmp/aprova",
		Linguagens:          map[sandbox.Linguagem]ExecutorDeLinguagem{sandbox.LinguagemGo: c.linguagem},
		Agora:               relogio.agora,
	}.Executar(t.Context())

	resultado := c.resultadoPublicado(t)
	if codigo != 0 || resultado.TipoFalha != sandbox.FalhaInfraestrutura {
		t.Fatalf("troca negada é infraestrutura entregue, obtive código %d e %+v", codigo, resultado)
	}
	if !strings.HasPrefix(resultado.SaidaBruta, "aprova: não foi possível rodar os testes como usuário sem privilégio") {
		t.Errorf("a mensagem precisa nomear a causa, não o passo em que ela apareceu: %q", resultado.SaidaBruta)
	}
	if !strings.Contains(resultado.SaidaBruta, "uid 10001): fork/exec /usr/bin/git: operation not permitted") {
		t.Errorf("o detalhe técnico precisa acompanhar a mensagem: %q", resultado.SaidaBruta)
	}
	if negador.tentativas != 1 || c.linguagem.chamadas != 0 {
		t.Errorf("nada roda depois da troca negada: %d tentativas, %d execuções da linguagem", negador.tentativas, c.linguagem.chamadas)
	}
}

func (c *cenarioDoPrograma) executarEm(t *testing.T, diretorioDeTrabalho string) int {
	t.Helper()

	relogio := &relogioDeTeste{}
	return Programa{
		LerVariavel:         func(nome string) string { return c.variaveis[nome] },
		Saida:               &c.saida,
		Rodador:             c.git,
		DiretorioDeTrabalho: diretorioDeTrabalho,
		Linguagens:          map[sandbox.Linguagem]ExecutorDeLinguagem{sandbox.LinguagemGo: c.linguagem},
		Agora:               relogio.agora,
	}.Executar(t.Context())
}

func criarDiretorio(t *testing.T, partes ...string) string {
	t.Helper()

	caminho := filepath.Join(partes...)
	if err := os.MkdirAll(caminho, 0o755); err != nil {
		t.Fatal(err)
	}
	return caminho
}

func TestProgramaRodaALinguagemNoDiretorioDoProjeto(t *testing.T) {
	trabalho := t.TempDir()
	projeto := criarDiretorio(t, trabalho, "repositorio", "go")
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_DIRETORIO"] = "go"

	c.executarEm(t, trabalho)

	if c.linguagem.diretorio != projeto {
		t.Errorf("a linguagem rodou em %q, esperava %q", c.linguagem.diretorio, projeto)
	}
	if resultado := c.resultadoPublicado(t); resultado.TipoFalha != sandbox.FalhaNenhuma {
		t.Errorf("obtive %+v", resultado)
	}
}

func TestProgramaComDiretorioDoProjetoInexistentePublicaInfraestrutura(t *testing.T) {
	trabalho := t.TempDir()
	criarDiretorio(t, trabalho, "repositorio")
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_DIRETORIO"] = "go"

	c.executarEm(t, trabalho)

	resultado := c.resultadoPublicado(t)
	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "go") {
		t.Errorf("obtive %+v", resultado)
	}
	if c.linguagem.chamadas != 0 {
		t.Error("sem o diretório do projeto nada é testado")
	}
}

func TestProgramaRecusaDiretorioDoProjetoQueApontaParaForaDoRepositorio(t *testing.T) {
	trabalho := t.TempDir()
	repositorio := criarDiretorio(t, trabalho, "repositorio")
	forasteiro := criarDiretorio(t, t.TempDir(), "modulo-alheio")
	if err := os.Symlink(forasteiro, filepath.Join(repositorio, "go")); err != nil {
		t.Fatal(err)
	}
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_DIRETORIO"] = "go"

	c.executarEm(t, trabalho)

	resultado := c.resultadoPublicado(t)
	if resultado.TipoFalha != sandbox.FalhaInfraestrutura || !strings.Contains(resultado.SaidaBruta, "fora do repositório") {
		t.Errorf("um link simbólico do PR não pode levar os testes para fora do clone, obtive %+v", resultado)
	}
	if c.linguagem.chamadas != 0 {
		t.Error("nada pode ser testado fora do repositório")
	}
}

func TestProgramaRecusaDiretorioDoProjetoQueEArquivo(t *testing.T) {
	trabalho := t.TempDir()
	repositorio := criarDiretorio(t, trabalho, "repositorio")
	if err := os.WriteFile(filepath.Join(repositorio, "go"), []byte("não sou diretório"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := novoCenarioDoPrograma()
	c.variaveis["APROVA_DIRETORIO"] = "go"

	c.executarEm(t, trabalho)

	if resultado := c.resultadoPublicado(t); resultado.TipoFalha != sandbox.FalhaInfraestrutura || c.linguagem.chamadas != 0 {
		t.Errorf("obtive %+v com %d execuções", resultado, c.linguagem.chamadas)
	}
}
