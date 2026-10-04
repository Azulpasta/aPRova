package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azulpasta/aPRova/internal/platform"
)

func TestLerArgumentosAceitaConsultaCompleta(t *testing.T) {
	obtida, err := lerArgumentos([]string{"consultar-permissao", "7001", "Azulpasta/aPRova", "erin"})

	if err != nil {
		t.Fatalf("argumentos válidos não deveriam falhar: %v", err)
	}
	esperada := consulta{instalacaoID: 7001, dono: "Azulpasta", repositorio: "aPRova", login: "erin"}
	if obtida != esperada {
		t.Errorf("obtive %+v, esperava %+v", obtida, esperada)
	}
}

func TestLerArgumentosRejeitaEntradaIncompletaOuMalformada(t *testing.T) {
	casos := map[string][]string{
		"sem argumentos":             {"consultar-permissao"},
		"sem login":                  {"consultar-permissao", "7001", "Azulpasta/aPRova"},
		"argumento a mais":           {"consultar-permissao", "7001", "Azulpasta/aPRova", "erin", "extra"},
		"instalação não numérica":    {"consultar-permissao", "sete", "Azulpasta/aPRova", "erin"},
		"repositório sem dono":       {"consultar-permissao", "7001", "aPRova", "erin"},
		"repositório com dono vazio": {"consultar-permissao", "7001", "/aPRova", "erin"},
		"login vazio":                {"consultar-permissao", "7001", "Azulpasta/aPRova", " "},
	}

	for nome, argumentos := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := lerArgumentos(argumentos); err == nil {
				t.Error("esperava erro explicando o uso")
			}
		})
	}
}

func TestDescreverPermissaoDizSePodeAprovar(t *testing.T) {
	casos := map[platform.Permissao]string{
		platform.PermissaoAdmin:  "pode aprovar",
		platform.PermissaoWrite:  "pode aprovar",
		platform.PermissaoTriage: "não pode aprovar",
		platform.PermissaoRead:   "não pode aprovar",
	}

	for permissao, trecho := range casos {
		descricao := descreverPermissao(consulta{dono: "Azulpasta", repositorio: "aPRova", login: "erin"}, permissao)

		if !strings.Contains(descricao, string(permissao)) || !strings.Contains(descricao, trecho) {
			t.Errorf("descrição de %s deveria conter %q: %s", permissao, trecho, descricao)
		}
	}
}

func TestExplicarFalhaOrientaQuandoOLoginNaoEColaborador(t *testing.T) {
	err := explicarFalha(platform.ErrNaoEncontrado)

	if !errors.Is(err, platform.ErrNaoEncontrado) || !strings.Contains(err.Error(), "installation_id") {
		t.Errorf("a orientação precisa preservar o erro e apontar a causa provável: %v", err)
	}
}
