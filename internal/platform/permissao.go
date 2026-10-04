package platform

import (
	"fmt"
	"slices"
)

// Permissao é o papel de um usuário num repositório, como o GitHub o nomeia.
type Permissao string

// Papéis que o GitHub atribui a colaboradores, do mais amplo ao mais restrito.
const (
	PermissaoAdmin    Permissao = "admin"
	PermissaoMaintain Permissao = "maintain"
	PermissaoWrite    Permissao = "write"
	PermissaoTriage   Permissao = "triage"
	PermissaoRead     Permissao = "read"
	PermissaoNenhuma  Permissao = "none"
)

var permissoesReconhecidas = []Permissao{
	PermissaoAdmin,
	PermissaoMaintain,
	PermissaoWrite,
	PermissaoTriage,
	PermissaoRead,
	PermissaoNenhuma,
}

func interpretarPermissao(papelDetalhado, permissaoBase string) (Permissao, error) {
	if slices.Contains(permissoesReconhecidas, Permissao(papelDetalhado)) {
		return Permissao(papelDetalhado), nil
	}

	if slices.Contains(permissoesReconhecidas, Permissao(permissaoBase)) {
		return Permissao(permissaoBase), nil
	}

	return "", fmt.Errorf("%w: role_name %q, permission %q", ErrPermissaoDesconhecida, papelDetalhado, permissaoBase)
}
