package main

import (
	"strings"
	"testing"
	"time"
)

const shaDoFrete = "9b1f4e2c7a3d5b8e0f6a1c2d3e4f5a6b7c8d9e0f"

func TestLerArgumentosAceitaRepositorioESHA(t *testing.T) {
	obtido, err := lerArgumentos([]string{"disparar", "https://github.com/Azulpasta/loja.git", shaDoFrete})

	if err != nil {
		t.Fatalf("argumentos válidos não deveriam falhar: %v", err)
	}
	if obtido.repositorio != "https://github.com/Azulpasta/loja.git" || obtido.sha != shaDoFrete {
		t.Errorf("obtive %+v", obtido)
	}
}

func TestLerArgumentosAceitaDiretorioDoProjetoOpcional(t *testing.T) {
	obtido, err := lerArgumentos([]string{"disparar", "https://github.com/Azulpasta/aPRova-TESTES.git", shaDoFrete, "go"})

	if err != nil || obtido.diretorio != "go" {
		t.Errorf("esperava o diretório go, obtive %+v e %v", obtido, err)
	}
}

func TestLerArgumentosRejeitaEntradaIncompleta(t *testing.T) {
	casos := map[string][]string{
		"sem argumentos":      {"disparar"},
		"sem SHA":             {"disparar", "https://github.com/Azulpasta/loja.git"},
		"argumento a mais":    {"disparar", "https://github.com/Azulpasta/loja.git", shaDoFrete, "go", "extra"},
		"diretório que sai":   {"disparar", "https://github.com/Azulpasta/loja.git", shaDoFrete, "../fora"},
		"SHA abreviado":       {"disparar", "https://github.com/Azulpasta/loja.git", "9b1f4e2"},
		"repositório sem URL": {"disparar", "Azulpasta/loja", shaDoFrete},
	}

	for nome, argumentos := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := lerArgumentos(argumentos); err == nil {
				t.Error("esperava erro explicando o uso")
			}
		})
	}
}

func TestVariaveisDoDisparoPedemOResultadoNoStdout(t *testing.T) {
	momento := time.Date(2026, 10, 7, 15, 4, 5, 0, time.UTC)

	variaveis := variaveisDoDisparo(disparo{repositorio: "https://github.com/Azulpasta/loja.git", sha: shaDoFrete, diretorio: "go"}, momento)

	esperadas := map[string]string{
		"APROVA_REPOSITORIO":    "https://github.com/Azulpasta/loja.git",
		"APROVA_SHA":            shaDoFrete,
		"APROVA_LINGUAGEM":      "go",
		"APROVA_TASK_RUN_ID":    "verificacao-manual-20261007T150405Z",
		"APROVA_PRAZO_SEGUNDOS": "600",
		"APROVA_SAIDA":          "stdout",
		"APROVA_DIRETORIO":      "go",
	}
	for nome, valor := range esperadas {
		if variaveis[nome] != valor {
			t.Errorf("%s = %q, esperava %q", nome, variaveis[nome], valor)
		}
	}
	if len(variaveis) != len(esperadas) {
		t.Errorf("variáveis a mais: %v", variaveis)
	}
}

func TestDisparoSemDiretorioUsaARaiz(t *testing.T) {
	pedido, err := lerArgumentos([]string{"disparar", "https://github.com/Azulpasta/loja.git", shaDoFrete})
	if err != nil {
		t.Fatalf("argumentos válidos não deveriam falhar: %v", err)
	}

	variaveis := variaveisDoDisparo(pedido, time.Now())

	if diretorio, presente := variaveis["APROVA_DIRETORIO"]; !presente || diretorio != "" {
		t.Errorf("sem o quarto argumento, APROVA_DIRETORIO vai vazio, a raiz; obtive %q (presente=%v)", diretorio, presente)
	}
}

func TestExigirImagemRecusaImagemSemTagOuComLatest(t *testing.T) {
	for _, imagem := range []string{"", "aprova.azurecr.io/aprova-sandbox-go", "aprova.azurecr.io/aprova-sandbox-go:latest"} {
		if err := exigirImagem(imagem); err == nil || !strings.Contains(err.Error(), "SANDBOX_IMAGEM_GO") {
			t.Errorf("imagem %q deveria ser recusada citando SANDBOX_IMAGEM_GO, obtive %v", imagem, err)
		}
	}

	if err := exigirImagem("aprova.azurecr.io/aprova-sandbox-go:" + shaDoFrete); err != nil {
		t.Errorf("imagem com tag de SHA é válida: %v", err)
	}
}
