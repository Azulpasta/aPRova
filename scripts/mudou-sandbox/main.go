package main

import (
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
	fmt.Println(decidir(os.Stdin))
}

func decidir(entrada io.Reader) bool {
	conteudo, err := io.ReadAll(entrada)
	if err != nil {
		fmt.Fprintf(os.Stderr, "não foi possível ler as mudanças, a integração roda: %v\n", err)
		return true
	}
	return afetaOSandbox(lerCaminhos(string(conteudo)))
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

func lerCaminhos(conteudo string) []string {
	var caminhos []string
	for _, trecho := range strings.FieldsFunc(conteudo, separaCaminhos) {
		if caminho := strings.TrimSpace(trecho); caminho != "" {
			caminhos = append(caminhos, caminho)
		}
	}
	return caminhos
}

func separaCaminhos(caractere rune) bool {
	return caractere == 0 || caractere == '\n'
}
