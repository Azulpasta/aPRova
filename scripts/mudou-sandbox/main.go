package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

var diretoriosDoSandbox = []string{
	"internal/sandbox/",
	"imagens/go/",
	"cmd/sandbox/",
	".github/workflows/",
	"tests/integration/",
}

var arquivosDoSandbox = []string{
	"go.mod",
	"go.sum",
	".dockerignore",
}

func main() {
	fmt.Println(afetaOSandbox(lerCaminhos(os.Stdin)))
}

func afetaOSandbox(caminhos []string) bool {
	return slices.ContainsFunc(caminhos, caminhoDoSandbox)
}

func caminhoDoSandbox(caminho string) bool {
	if slices.Contains(arquivosDoSandbox, caminho) {
		return true
	}
	return slices.ContainsFunc(diretoriosDoSandbox, func(diretorio string) bool {
		return strings.HasPrefix(caminho, diretorio)
	})
}

func lerCaminhos(entrada io.Reader) []string {
	var caminhos []string

	linhas := bufio.NewScanner(entrada)
	for linhas.Scan() {
		if caminho := strings.TrimSpace(linhas.Text()); caminho != "" {
			caminhos = append(caminhos, caminho)
		}
	}
	return caminhos
}
