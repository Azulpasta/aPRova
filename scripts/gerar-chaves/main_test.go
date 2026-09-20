package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestGerarParEmBase64ProduzChavesQueAssinamEVerificam(t *testing.T) {
	privadaEmBase64, publicaEmBase64, err := gerarParEmBase64()
	if err != nil {
		t.Fatalf("gerar par: %v", err)
	}

	privada, err := base64.StdEncoding.DecodeString(privadaEmBase64)
	if err != nil {
		t.Fatalf("a chave privada impressa não é base64 válido: %v", err)
	}
	publica, err := base64.StdEncoding.DecodeString(publicaEmBase64)
	if err != nil {
		t.Fatalf("a chave pública impressa não é base64 válido: %v", err)
	}

	if len(privada) != ed25519.PrivateKeySize {
		t.Errorf("chave privada deveria ter %d bytes, tem %d", ed25519.PrivateKeySize, len(privada))
	}
	if len(publica) != ed25519.PublicKeySize {
		t.Errorf("chave pública deveria ter %d bytes, tem %d", ed25519.PublicKeySize, len(publica))
	}

	mensagem := []byte("recibo de exemplo")
	assinatura := ed25519.Sign(ed25519.PrivateKey(privada), mensagem)

	if !ed25519.Verify(ed25519.PublicKey(publica), mensagem, assinatura) {
		t.Error("o par gerado deveria assinar e verificar a mesma mensagem")
	}
}

func TestGerarParEmBase64NaoRepeteChaves(t *testing.T) {
	primeira, _, err := gerarParEmBase64()
	if err != nil {
		t.Fatalf("gerar par: %v", err)
	}
	segunda, _, err := gerarParEmBase64()
	if err != nil {
		t.Fatalf("gerar par: %v", err)
	}

	if primeira == segunda {
		t.Error("duas execuções não podem produzir a mesma chave privada")
	}
}
