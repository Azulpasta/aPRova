package acaoremota

import (
	"testing"

	"github.com/Azulpasta/aPRova/internal/platform"
)

func TestPoliticaExigeEscritaParaCadaTipo(t *testing.T) {
	casos := []struct {
		tipo      Tipo
		permissao platform.Permissao
		autoriza  bool
	}{
		{TipoAprovar, platform.PermissaoAdmin, true},
		{TipoAprovar, platform.PermissaoMaintain, true},
		{TipoAprovar, platform.PermissaoWrite, true},
		{TipoAprovar, platform.PermissaoTriage, false},
		{TipoAprovar, platform.PermissaoRead, false},
		{TipoAprovar, platform.PermissaoNenhuma, false},
		{TipoAprovar, "", false},
		{TipoSolicitarAlteracao, platform.PermissaoAdmin, true},
		{TipoSolicitarAlteracao, platform.PermissaoMaintain, true},
		{TipoSolicitarAlteracao, platform.PermissaoWrite, true},
		{TipoSolicitarAlteracao, platform.PermissaoTriage, false},
		{TipoSolicitarAlteracao, platform.PermissaoRead, false},
		{TipoReexecutarAnalise, platform.PermissaoAdmin, true},
		{TipoReexecutarAnalise, platform.PermissaoMaintain, true},
		{TipoReexecutarAnalise, platform.PermissaoWrite, true},
		{TipoReexecutarAnalise, platform.PermissaoTriage, false},
		{TipoReexecutarAnalise, platform.PermissaoRead, false},
	}

	politica := NovaPolitica()

	for _, caso := range casos {
		t.Run(string(caso.tipo)+"/"+string(caso.permissao), func(t *testing.T) {
			veredito := politica.Avaliar(Situacao{
				Tipo:      caso.tipo,
				Permissao: caso.permissao,
				Login:     "erin",
				AutorDoPR: "carol",
			})

			if veredito.Autorizada != caso.autoriza {
				t.Errorf("autorizada = %v, esperava %v", veredito.Autorizada, caso.autoriza)
			}
			if !caso.autoriza && veredito.Motivo != MotivoPermissaoInsuficiente {
				t.Errorf("motivo = %q, esperava %q", veredito.Motivo, MotivoPermissaoInsuficiente)
			}
			if caso.autoriza && veredito.Motivo != MotivoAutorizada {
				t.Errorf("motivo = %q, esperava %q", veredito.Motivo, MotivoAutorizada)
			}
		})
	}
}

func TestPoliticaRecusaTipoSemRegraDefinida(t *testing.T) {
	veredito := NovaPolitica().Avaliar(Situacao{
		Tipo:      "mesclar",
		Permissao: platform.PermissaoAdmin,
		Login:     "erin",
	})

	if veredito.Autorizada {
		t.Error("tipo sem regra definida precisa ser recusado, mesmo para admin")
	}
}

func TestPoliticaImpedeAutorDeAprovarOProprioPR(t *testing.T) {
	casos := []struct {
		nome      string
		login     string
		autorDoPR string
	}{
		{"mesma grafia", "carol", "carol"},
		{"grafia diferente, mesmo login", "Carol", "carol"},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			veredito := NovaPolitica().Avaliar(Situacao{
				Tipo:      TipoAprovar,
				Permissao: platform.PermissaoAdmin,
				Login:     caso.login,
				AutorDoPR: caso.autorDoPR,
			})

			if veredito.Autorizada {
				t.Fatal("o autor não pode aprovar o próprio PR, nem sendo admin")
			}
			if veredito.Motivo != MotivoAutorDoProprioPR {
				t.Errorf("motivo = %q, esperava %q", veredito.Motivo, MotivoAutorDoProprioPR)
			}
		})
	}
}

func TestPoliticaPermiteAutorPedirReanaliseDoProprioPR(t *testing.T) {
	veredito := NovaPolitica().Avaliar(Situacao{
		Tipo:      TipoReexecutarAnalise,
		Permissao: platform.PermissaoWrite,
		Login:     "carol",
		AutorDoPR: "carol",
	})

	if !veredito.Autorizada {
		t.Error("a restrição de autoria vale só para aprovar")
	}
}
