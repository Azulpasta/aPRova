package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	privada, publica, err := gerarParEmBase64()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gerar par Ed25519: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Cole no seu .env:")
	fmt.Println()
	fmt.Printf("RECEIPT_PRIVATE_KEY_BASE64=%s\n", privada)
	fmt.Printf("RECEIPT_PUBLIC_KEY_BASE64=%s\n", publica)
	fmt.Println()
	fmt.Println("A chave privada é segredo. A pública deve ser publicada,")
	fmt.Println("senão ninguém consegue verificar os recibos de forma independente.")
}

func gerarParEmBase64() (privadaEmBase64, publicaEmBase64 string, err error) {
	publica, privada, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", fmt.Errorf("gerar chave ed25519: %w", err)
	}

	return base64.StdEncoding.EncodeToString(privada),
		base64.StdEncoding.EncodeToString(publica),
		nil
}
