package sandbox

const (
	codigoSucesso           = 0
	codigoFalhaDeTeste      = 1
	codigoFalhaDeCompilacao = 2
)

func classificarCodigoDeSaida(codigo int) (tipo TipoFalha, compilou bool) {
	switch codigo {
	case codigoSucesso:
		return FalhaNenhuma, true
	case codigoFalhaDeTeste:
		return FalhaTeste, true
	case codigoFalhaDeCompilacao:
		return FalhaCompilacao, false
	default:
		return FalhaInfraestrutura, false
	}
}

func resultadoDeInfraestrutura(saida string, duracaoMS int64) ResultadoSandbox {
	return ResultadoSandbox{SaidaBruta: saida, DuracaoMS: duracaoMS, TipoFalha: FalhaInfraestrutura}
}
