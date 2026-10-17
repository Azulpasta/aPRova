package conteiner

import (
	"strings"
	"testing"
	"time"

	"github.com/Azulpasta/aPRova/internal/sandbox"
)

const shaDeTeste = "0123456789abcdef0123456789abcdef01234567"

func variaveisValidas() map[string]string {
	return map[string]string{
		"APROVA_REPOSITORIO":       "https://github.com/Azulpasta/loja.git",
		"APROVA_SHA":               shaDeTeste,
		"APROVA_LINGUAGEM":         "go",
		"APROVA_TASK_RUN_ID":       "task-run-1",
		"APROVA_PRAZO_SEGUNDOS":    "600",
		"APROVA_SAIDA":             "stdout",
		"VARIAVEL_QUE_NAO_IMPORTA": "x",
	}
}

func leitorDe(variaveis map[string]string) func(string) string {
	return func(nome string) string { return variaveis[nome] }
}

func TestLerAmbienteComTudoValido(t *testing.T) {
	ambiente, err := LerAmbiente(leitorDe(variaveisValidas()))

	if err != nil {
		t.Fatalf("ambiente válido não deveria falhar: %v", err)
	}
	esperado := Ambiente{
		Repositorio: "https://github.com/Azulpasta/loja.git",
		SHA:         shaDeTeste,
		Linguagem:   sandbox.LinguagemGo,
		TaskRunID:   "task-run-1",
		Prazo:       10 * time.Minute,
	}
	if ambiente != esperado {
		t.Errorf("obtive %+v, esperava %+v", ambiente, esperado)
	}
}

func TestLerAmbienteNormalizaOSHAParaMinusculas(t *testing.T) {
	variaveis := variaveisValidas()
	variaveis["APROVA_SHA"] = strings.ToUpper(shaDeTeste)

	ambiente, err := LerAmbiente(leitorDe(variaveis))

	if err != nil || ambiente.SHA != shaDeTeste {
		t.Errorf("esperava o SHA em minúsculas, obtive %q e %v", ambiente.SHA, err)
	}
}

func TestLerAmbienteAceitaRepositorioLocalPorFile(t *testing.T) {
	variaveis := variaveisValidas()
	variaveis["APROVA_REPOSITORIO"] = "file:///repositorios/caso.git"

	if _, err := LerAmbiente(leitorDe(variaveis)); err != nil {
		t.Errorf("file:// serve aos testes de integração sem rede: %v", err)
	}
}

func TestLerAmbienteRecusaCadaVariavelAusenteOuInvalida(t *testing.T) {
	casos := []struct {
		variavel string
		valor    string
	}{
		{"APROVA_REPOSITORIO", ""},
		{"APROVA_REPOSITORIO", "github.com/sem-esquema"},
		{"APROVA_REPOSITORIO", "ssh://git@github.com/Azulpasta/loja.git"},
		{"APROVA_REPOSITORIO", "https://usuario:senha@github.com/Azulpasta/loja.git"},
		{"APROVA_SHA", ""},
		{"APROVA_SHA", "main"},
		{"APROVA_SHA", "0123456"},
		{"APROVA_SHA", strings.Repeat("g", 40)},
		{"APROVA_LINGUAGEM", ""},
		{"APROVA_LINGUAGEM", "rust"},
		{"APROVA_TASK_RUN_ID", ""},
		{"APROVA_PRAZO_SEGUNDOS", ""},
		{"APROVA_PRAZO_SEGUNDOS", "dez"},
		{"APROVA_PRAZO_SEGUNDOS", "0"},
		{"APROVA_PRAZO_SEGUNDOS", "-60"},
		{"APROVA_PRAZO_SEGUNDOS", "86401"},
	}

	for _, caso := range casos {
		t.Run(caso.variavel+"="+caso.valor, func(t *testing.T) {
			variaveis := variaveisValidas()
			variaveis[caso.variavel] = caso.valor

			_, err := LerAmbiente(leitorDe(variaveis))

			if err == nil || !strings.Contains(err.Error(), caso.variavel) {
				t.Errorf("esperava erro nomeando %s, obtive %v", caso.variavel, err)
			}
		})
	}
}

func TestLerAmbienteNomeiaTodasAsVariaveisComProblema(t *testing.T) {
	_, err := LerAmbiente(leitorDe(map[string]string{}))

	if err == nil {
		t.Fatal("ambiente vazio precisa falhar")
	}
	for _, variavel := range []string{"APROVA_REPOSITORIO", "APROVA_SHA", "APROVA_LINGUAGEM", "APROVA_TASK_RUN_ID", "APROVA_PRAZO_SEGUNDOS"} {
		if !strings.Contains(err.Error(), variavel) {
			t.Errorf("o erro precisa citar %s: %v", variavel, err)
		}
	}
}
