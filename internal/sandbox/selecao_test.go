package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func opcoesAzureCompletas() OpcoesExecutorAzure {
	return OpcoesExecutorAzure{
		SubscriptionID: "00000000-0000-0000-0000-000000000001",
		TenantID:       "00000000-0000-0000-0000-000000000002",
		ClientID:       "00000000-0000-0000-0000-000000000003",
		ClientSecret:   "segredo-de-teste",
		ResourceGroup:  "rg-aprova",
		NomeDoJob:      "job-sandbox",
	}
}

func TestExecutorDeSandboxInvalidoEErroDeConfiguracao(t *testing.T) {
	for _, tipo := range []string{"", "kubernetes", "cloudrun", "LOCAL"} {
		t.Run("tipo "+tipo, func(t *testing.T) {
			_, err := NovoExecutor(ConfiguracaoExecutor{Tipo: tipo})

			if !errors.Is(err, ErrConfiguracaoSandbox) {
				t.Fatalf("esperava ErrConfiguracaoSandbox, obtive %v", err)
			}
			if !strings.Contains(err.Error(), "SANDBOX_EXECUTOR") {
				t.Errorf("a mensagem deve nomear a variável, obtive: %v", err)
			}
		})
	}
}

func TestExecutorLocalEConstruido(t *testing.T) {
	executor, err := NovoExecutor(ConfiguracaoExecutor{Tipo: "local", Local: opcoesLocaisDeTeste()})

	if err != nil {
		t.Fatalf("executor local deveria ser construído: %v", err)
	}
	if executor == nil {
		t.Fatal("executor nulo")
	}
}

func TestExecutorAzureSemCredencialNomeiaCadaVariavelAusente(t *testing.T) {
	opcoes := opcoesAzureCompletas()
	opcoes.TenantID = ""
	opcoes.NomeDoJob = ""

	_, err := NovoExecutor(ConfiguracaoExecutor{Tipo: "azure", Azure: opcoes, Coletor: NovoColetorEmMemoria()})

	if !errors.Is(err, ErrConfiguracaoSandbox) {
		t.Fatalf("esperava ErrConfiguracaoSandbox, obtive %v", err)
	}
	for _, variavel := range []string{"AZURE_TENANT_ID", "AZURE_SANDBOX_JOB_NAME"} {
		if !strings.Contains(err.Error(), variavel) {
			t.Errorf("a mensagem deveria nomear %s, obtive: %v", variavel, err)
		}
	}
	if strings.Contains(err.Error(), "AZURE_CLIENT_ID") {
		t.Errorf("AZURE_CLIENT_ID está preenchida e não deveria aparecer: %v", err)
	}
}

func TestExecutorAzureSemColetorEErroDeConfiguracao(t *testing.T) {
	_, err := NovoExecutor(ConfiguracaoExecutor{Tipo: "azure", Azure: opcoesAzureCompletas()})

	if !errors.Is(err, ErrConfiguracaoSandbox) {
		t.Errorf("sem coletor o resultado do job nunca chega; esperava ErrConfiguracaoSandbox, obtive %v", err)
	}
}

func TestExecutorAzureCompletoEConstruidoSemTocarARede(t *testing.T) {
	executor, err := NovoExecutor(ConfiguracaoExecutor{
		Tipo:    "azure",
		Azure:   opcoesAzureCompletas(),
		Coletor: NovoColetorEmMemoria(),
	})

	if err != nil {
		t.Fatalf("configuração completa deveria construir o executor: %v", err)
	}
	if executor == nil {
		t.Fatal("executor nulo")
	}
}
