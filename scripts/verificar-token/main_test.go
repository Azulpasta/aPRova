package main

import "testing"

func TestLerInstalacaoIDAceitaNumeroValido(t *testing.T) {
	obtido, err := lerInstalacaoID([]string{"verificar-token", "12345678"})
	if err != nil {
		t.Fatalf("identificador válido não deveria falhar: %v", err)
	}
	if obtido != 12345678 {
		t.Errorf("esperava 12345678, obtive %d", obtido)
	}
}

func TestLerInstalacaoIDExigeArgumento(t *testing.T) {
	_, err := lerInstalacaoID([]string{"verificar-token"})
	if err == nil {
		t.Fatal("sem argumento deveria falhar explicando o uso")
	}
}

func TestLerInstalacaoIDRejeitaValorNaoNumerico(t *testing.T) {
	_, err := lerInstalacaoID([]string{"verificar-token", "a-instalacao"})
	if err == nil {
		t.Fatal("valor não numérico deveria falhar")
	}
}
