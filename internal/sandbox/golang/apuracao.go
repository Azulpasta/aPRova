package golang

import (
	"bytes"
	"encoding/json"
	"strings"
)

const (
	acaoPassou            = "pass"
	acaoFalhou            = "fail"
	acaoFalhaDeCompilacao = "build-fail"
	separadorDeSubteste   = "/"
)

type eventoDeTeste struct {
	Acao             string `json:"Action"`
	Pacote           string `json:"Package"`
	Teste            string `json:"Test"`
	Saida            string `json:"Output"`
	CompilacaoFalhou string `json:"FailedBuild"`
}

type apuracao struct {
	passaram          int
	falharam          int
	falhaDeCompilacao bool
	saida             string
}

type apurador struct {
	resultado          apuracao
	saida              strings.Builder
	falhasPorPacote    map[string]int
	pacotesQueFalharam []string
}

func apurar(saidaDoGoTest []byte) apuracao {
	apurando := &apurador{falhasPorPacote: map[string]int{}}

	for linha := range bytes.Lines(saidaDoGoTest) {
		apurando.considerar(linha)
	}

	return apurando.concluir()
}

func (a *apurador) considerar(linha []byte) {
	var evento eventoDeTeste
	if err := json.Unmarshal(linha, &evento); err != nil {
		a.saida.Write(linha)
		if !bytes.HasSuffix(linha, []byte("\n")) {
			a.saida.WriteByte('\n')
		}
		return
	}

	a.saida.WriteString(evento.Saida)

	switch {
	case evento.Acao == acaoFalhaDeCompilacao:
		a.resultado.falhaDeCompilacao = true
	case evento.Teste == "" && evento.Acao == acaoFalhou:
		a.registrarFalhaDePacote(evento)
	case ehTesteDeNivelSuperior(evento.Teste):
		a.contarTeste(evento)
	}
}

func (a *apurador) contarTeste(evento eventoDeTeste) {
	switch evento.Acao {
	case acaoPassou:
		a.resultado.passaram++
	case acaoFalhou:
		a.resultado.falharam++
		a.falhasPorPacote[evento.Pacote]++
	}
}

func (a *apurador) registrarFalhaDePacote(evento eventoDeTeste) {
	if evento.CompilacaoFalhou != "" {
		a.resultado.falhaDeCompilacao = true
		return
	}
	a.pacotesQueFalharam = append(a.pacotesQueFalharam, evento.Pacote)
}

func (a *apurador) concluir() apuracao {
	for _, pacote := range a.pacotesQueFalharam {
		if a.falhasPorPacote[pacote] == 0 {
			a.resultado.falharam++
		}
	}

	a.resultado.saida = a.saida.String()
	return a.resultado
}

func ehTesteDeNivelSuperior(teste string) bool {
	return teste != "" && !strings.Contains(teste, separadorDeSubteste)
}
