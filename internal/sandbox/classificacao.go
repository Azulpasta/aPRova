package sandbox

import (
	"encoding/json"
	"slices"
	"strings"
)

var tiposDeFalhaConhecidos = []TipoFalha{FalhaNenhuma, FalhaTeste, FalhaCompilacao, FalhaInfraestrutura}

func interpretarResultadoPublicado(stdout string) (ResultadoSandbox, bool) {
	linhas := strings.Split(strings.TrimSpace(stdout), "\n")
	ultima := linhas[len(linhas)-1]

	var resultado ResultadoSandbox
	if err := json.Unmarshal([]byte(ultima), &resultado); err != nil {
		return ResultadoSandbox{}, false
	}
	if !slices.Contains(tiposDeFalhaConhecidos, resultado.TipoFalha) {
		return ResultadoSandbox{}, false
	}
	return resultado, true
}

func resultadoDeInfraestrutura(saida string, duracaoMS int64) ResultadoSandbox {
	return ResultadoSandbox{SaidaBruta: saida, DuracaoMS: duracaoMS, TipoFalha: FalhaInfraestrutura}
}
