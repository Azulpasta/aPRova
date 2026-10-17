package frete

import "testing"

func TestFreteLeve(t *testing.T) {
	if Calcular(1, 2) != 3 {
		t.Fatal("frete leve errado")
	}
}

func TestFretePesado(t *testing.T) {
	if Calcular(10, 5) != 15 {
		t.Fatal("frete pesado errado")
	}
}
