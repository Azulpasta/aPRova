package sandbox

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// ErrDiretorioInvalido indica diretório do projeto que sai do repositório ou é
// ambíguo.
var ErrDiretorioInvalido = errors.New("sandbox: diretório do projeto inválido")

var (
	formatoDoDiretorio  = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
	segmentosReservados = []string{".", ".."}
)

// ValidarDiretorioDoProjeto confere o diretório do projeto dentro do
// repositório: vazio é a raiz; qualquer outro valor precisa ser um caminho
// relativo, já na forma canônica e sem segmentos . ou ..
func ValidarDiretorioDoProjeto(diretorio string) error {
	if diretorio == "" {
		return nil
	}

	valido := formatoDoDiretorio.MatchString(diretorio) &&
		path.Clean(diretorio) == diretorio &&
		!slices.ContainsFunc(strings.Split(diretorio, "/"), ehSegmentoReservado)
	if !valido {
		return fmt.Errorf("%w: %q precisa ser um caminho relativo dentro do repositório, como go ou servicos/frete", ErrDiretorioInvalido, diretorio)
	}
	return nil
}

func ehSegmentoReservado(segmento string) bool {
	return slices.Contains(segmentosReservados, segmento)
}
