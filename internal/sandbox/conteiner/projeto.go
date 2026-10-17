package conteiner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrProjetoInacessivel indica diretório do projeto ausente, que não é
// diretório, que passa por link simbólico ou que, resolvido, fica fora do
// repositório clonado.
var ErrProjetoInacessivel = errors.New("sandbox: diretório do projeto inacessível")

func localizarProjeto(repositorio, diretorio string) (string, error) {
	if diretorio == "" {
		return repositorio, nil
	}

	projeto := filepath.Join(repositorio, filepath.FromSlash(diretorio))
	if err := exigirSemLinks(repositorio, diretorio); err != nil {
		return "", err
	}

	raizReal, err := filepath.EvalSymlinks(repositorio)
	if err != nil {
		return "", fmt.Errorf("%w: repositório clonado não pôde ser resolvido: %w", ErrProjetoInacessivel, err)
	}
	projetoReal, err := filepath.EvalSymlinks(projeto)
	if err != nil {
		return "", fmt.Errorf("%w: %s não existe no repositório", ErrProjetoInacessivel, diretorio)
	}

	if !estaDentro(raizReal, projetoReal) {
		return "", fmt.Errorf("%w: %s aponta para fora do repositório", ErrProjetoInacessivel, diretorio)
	}

	informacao, err := os.Stat(projetoReal)
	if err != nil || !informacao.IsDir() {
		return "", fmt.Errorf("%w: %s não é um diretório", ErrProjetoInacessivel, diretorio)
	}

	return projeto, nil
}

func exigirSemLinks(repositorio, diretorio string) error {
	caminho := repositorio
	for _, componente := range strings.Split(diretorio, "/") {
		caminho = filepath.Join(caminho, componente)

		informacao, err := os.Lstat(caminho)
		if err != nil {
			return fmt.Errorf("%w: %s não existe no repositório", ErrProjetoInacessivel, diretorio)
		}
		if informacao.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s passa por um link simbólico; o diretório do projeto precisa ser um diretório real do repositório", ErrProjetoInacessivel, diretorio)
		}
	}
	return nil
}

func estaDentro(raiz, caminho string) bool {
	relativo, err := filepath.Rel(raiz, caminho)
	if err != nil {
		return false
	}
	return relativo != ".." && !strings.HasPrefix(relativo, ".."+string(filepath.Separator))
}
