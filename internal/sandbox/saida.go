package sandbox

import "unicode/utf8"

const limiteSaidaPadrao = 64 * 1024

func truncarMantendoFinal(saida string, limite int) string {
	if len(saida) <= limite {
		return saida
	}

	inicio := len(saida) - limite
	for inicio < len(saida) && !utf8.RuneStart(saida[inicio]) {
		inicio++
	}

	return saida[inicio:]
}

// LimitarSaida reduz a saída ao limite padrão do ResultadoSandbox, mantendo o
// final, onde costumam estar o erro e o resumo dos testes.
func LimitarSaida(saida string) string {
	return truncarMantendoFinal(saida, limiteSaidaPadrao)
}
