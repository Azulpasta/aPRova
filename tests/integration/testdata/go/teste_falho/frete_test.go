package frete

import "testing"

func TestFreteLeve(t *testing.T) {
	if Calcular(1, 2) != 3 {
		t.Fatal("frete leve errado")
	}
}

func TestFreteComTaxa(t *testing.T) {
	if Calcular(10, 5) != 16 {
		t.Fatalf("frete com taxa: esperava 16, obtive %d", Calcular(10, 5))
	}
}

func TestFreteZero(t *testing.T) {
	if Calcular(0, 0) != 0 {
		t.Fatal("frete zero errado")
	}
}
