package main

import (
	"strings"
	"testing"
)

func TestAfetaOSandboxReconheceCadaCaminhoMonitorado(t *testing.T) {
	afetam := []string{
		"internal/sandbox/executor_local.go",
		"internal/sandbox/golang/apuracao.go",
		"imagens/go/Dockerfile",
		"cmd/sandbox/main.go",
		"go.mod",
		"go.sum",
		".dockerignore",
		".github/workflows/ci.yml",
		".github/workflows/imagem-sandbox-go.yml",
		"tests/integration/sandbox_go_test.go",
		"tests/integration/testdata/go/ok/frete.go",
	}

	for _, caminho := range afetam {
		if !afetaOSandbox([]string{"README.md", caminho}) {
			t.Errorf("%s afeta o sandbox e precisa disparar a integração", caminho)
		}
	}
}

func TestAfetaOSandboxIgnoraOQueNaoMexeNele(t *testing.T) {
	naoAfetam := []string{
		"README.md",
		"internal/api/acoes_remotas.go",
		"internal/sandboxes.go",
		"cmd/api/main.go",
		"cmd/sandboxer/main.go",
		"imagens/python/Dockerfile",
		"docs/go.mod",
		"go.mod.bak",
		".github/CODEOWNERS",
		"tests/functional/acoes_remotas_test.go",
		"",
	}

	if afetaOSandbox(naoAfetam) {
		t.Errorf("nenhum destes caminhos afeta o sandbox: %v", naoAfetam)
	}
}

func TestLerCaminhosIgnoraLinhasVaziasEEspacos(t *testing.T) {
	caminhos := lerCaminhos(strings.NewReader("README.md\n\n  cmd/sandbox/main.go  \n"))

	if len(caminhos) != 2 || caminhos[1] != "cmd/sandbox/main.go" {
		t.Errorf("obtive %q", caminhos)
	}
}
