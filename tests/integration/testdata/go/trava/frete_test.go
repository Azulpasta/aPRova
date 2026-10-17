package frete

import (
	"testing"
	"time"
)

func TestFreteLeve(t *testing.T) {
	if Calcular(1, 2) != 3 {
		t.Fatal("frete leve errado")
	}
}

func TestFreteQueNuncaTermina(t *testing.T) {
	time.Sleep(time.Hour)
}
