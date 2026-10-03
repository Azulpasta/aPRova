package jobsazure

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v4"
)

func texto(valor string) *string {
	return &valor
}

func porNome(variaveis []*armappcontainers.EnvironmentVar) map[string]*armappcontainers.EnvironmentVar {
	indice := map[string]*armappcontainers.EnvironmentVar{}
	for _, variavel := range variaveis {
		indice[*variavel.Name] = variavel
	}
	return indice
}

func TestMesclaPreservaVariaveisQueOJobJaTinha(t *testing.T) {
	doJob := []*armappcontainers.EnvironmentVar{
		{Name: texto("DATABASE_URL"), SecretRef: texto("segredo-banco")},
		{Name: texto("LOG_LEVEL"), Value: texto("info")},
	}

	mescladas := porNome(mesclarVariaveis(doJob, map[string]string{"APROVA_SHA": "abc"}))

	if mescladas["DATABASE_URL"] == nil || *mescladas["DATABASE_URL"].SecretRef != "segredo-banco" {
		t.Error("a referência a segredo do job sumiu; o disparo apagaria a configuração do container")
	}
	if mescladas["LOG_LEVEL"] == nil || *mescladas["LOG_LEVEL"].Value != "info" {
		t.Error("variável comum do job deveria ser preservada")
	}
	if mescladas["APROVA_SHA"] == nil || *mescladas["APROVA_SHA"].Value != "abc" {
		t.Error("a variável do pedido deveria ser acrescentada")
	}
}

func TestMesclaSobrescreveVariavelDeMesmoNome(t *testing.T) {
	doJob := []*armappcontainers.EnvironmentVar{{Name: texto("APROVA_SHA"), Value: texto("valor-antigo")}}

	mescladas := mesclarVariaveis(doJob, map[string]string{"APROVA_SHA": "valor-do-pedido"})

	if len(mescladas) != 1 {
		t.Fatalf("variável de mesmo nome não pode duplicar; obtive %d entradas", len(mescladas))
	}
	if *mescladas[0].Value != "valor-do-pedido" {
		t.Errorf("o pedido deveria prevalecer, obtive %q", *mescladas[0].Value)
	}
}

func TestStatusTerminalReconheceQuandoAExecucaoAcabou(t *testing.T) {
	casos := map[string]bool{
		StatusConcluido:       true,
		StatusFalhou:          true,
		StatusParado:          true,
		StatusDegradado:       true,
		StatusRodando:         false,
		StatusEmProcessamento: false,
		StatusDesconhecido:    false,
	}

	for status, esperado := range casos {
		if obtido := StatusTerminal(status); obtido != esperado {
			t.Errorf("StatusTerminal(%q) deveria ser %v", status, esperado)
		}
	}
}
