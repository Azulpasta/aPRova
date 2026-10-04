package acaoremota

import (
	"slices"
	"strings"

	"github.com/Azulpasta/aPRova/internal/platform"
)

var permissoesDeEscrita = []platform.Permissao{
	platform.PermissaoWrite,
	platform.PermissaoMaintain,
	platform.PermissaoAdmin,
}

// Situacao reúne o que a política precisa saber para decidir um pedido.
type Situacao struct {
	Tipo      Tipo
	Permissao platform.Permissao
	Login     string
	AutorDoPR string
}

// Veredito é a resposta da política para uma situação.
type Veredito struct {
	Autorizada bool
	Motivo     Motivo
}

// Politica decide se um login pode executar uma ação num repositório.
type Politica struct {
	permissoesExigidas map[Tipo][]platform.Permissao
}

// NovaPolitica constrói a política com as regras do aPRova.
func NovaPolitica() Politica {
	return Politica{permissoesExigidas: map[Tipo][]platform.Permissao{
		TipoAprovar:            permissoesDeEscrita,
		TipoSolicitarAlteracao: permissoesDeEscrita,
		TipoReexecutarAnalise:  permissoesDeEscrita,
	}}
}

// Avaliar devolve o veredito para a situação informada. Tipo sem regra
// definida é sempre recusado.
func (p Politica) Avaliar(situacao Situacao) Veredito {
	if !slices.Contains(p.permissoesExigidas[situacao.Tipo], situacao.Permissao) {
		return recusar(MotivoPermissaoInsuficiente)
	}

	if situacao.Tipo == TipoAprovar && strings.EqualFold(situacao.Login, situacao.AutorDoPR) {
		return recusar(MotivoAutorDoProprioPR)
	}

	return Veredito{Autorizada: true, Motivo: MotivoAutorizada}
}

func recusar(motivo Motivo) Veredito {
	return Veredito{Autorizada: false, Motivo: motivo}
}
