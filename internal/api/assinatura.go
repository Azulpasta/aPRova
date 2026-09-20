package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	cabecalhoAssinatura = "X-Hub-Signature-256"
	prefixoAssinatura   = "sha256="
)

func assinaturaConfere(segredo, corpoBruto []byte, cabecalho string) bool {
	if !strings.HasPrefix(cabecalho, prefixoAssinatura) {
		return false
	}

	recebida, err := hex.DecodeString(strings.TrimPrefix(cabecalho, prefixoAssinatura))
	if err != nil {
		return false
	}

	calculada := hmac.New(sha256.New, segredo)
	calculada.Write(corpoBruto)

	return hmac.Equal(recebida, calculada.Sum(nil))
}
