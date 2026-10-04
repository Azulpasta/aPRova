package acaoremota

import (
	"strings"
	"testing"
)

func TestServicoRejeitaCorpoInvalidoSemConsultarNada(t *testing.T) {
	casos := []struct {
		nome  string
		corpo func(t *testing.T) []byte
	}{
		{"tipo desconhecido", comCampos(map[string]any{"tipo": "mesclar"})},
		{"tipo com grafia diferente", comCampos(map[string]any{"tipo": "Aprovar"})},
		{"solicitar alteração sem justificativa", comCampos(map[string]any{"tipo": "solicitar_alteracao"})},
		{"solicitar alteração com justificativa em branco", comCampos(map[string]any{"tipo": "solicitar_alteracao", "justificativa": "   \n\t"})},
		{"sem id_acao", comCampos(map[string]any{"id_acao": nil})},
		{"id_acao longo demais", comCampos(map[string]any{"id_acao": strings.Repeat("x", 201)})},
		{"sem usuario_slack", comCampos(map[string]any{"usuario_slack": nil})},
		{"sem repositorio", comCampos(map[string]any{"repositorio": nil})},
		{"repositorio sem dono", comCampos(map[string]any{"repositorio": "aPRova"})},
		{"repositorio com dono vazio", comCampos(map[string]any{"repositorio": "/aPRova"})},
		{"repositorio com nome vazio", comCampos(map[string]any{"repositorio": "Azulpasta/"})},
		{"repositorio com barra a mais", comCampos(map[string]any{"repositorio": "Azulpasta/aPRova/issues"})},
		{"repositorio que sobe diretório", comCampos(map[string]any{"repositorio": "Azulpasta/.."})},
		{"repositorio com espaço", comCampos(map[string]any{"repositorio": "Azul pasta/aPRova"})},
		{"repositorio com consulta embutida", comCampos(map[string]any{"repositorio": "Azulpasta/aPRova?x=1"})},
		{"sem numero_pr", comCampos(map[string]any{"numero_pr": nil})},
		{"numero_pr negativo", comCampos(map[string]any{"numero_pr": -1})},
		{"numero_pr como texto", comCampos(map[string]any{"numero_pr": "42"})},
		{"sem installation_id", comCampos(map[string]any{"installation_id": nil})},
		{"campo desconhecido", comCampos(map[string]any{"login_github": "admin"})},
		{"json malformado", func(*testing.T) []byte { return []byte(`{"id_acao":`) }},
		{"dois objetos seguidos", func(t *testing.T) []byte {
			return append(corpoDoPedido(t, nil), corpoDoPedido(t, map[string]any{"id_acao": "acao-2"})...)
		}},
		{"corpo vazio", func(*testing.T) []byte { return nil }},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			c := novoCenario(t)

			decisao := c.servico.Processar(t.Context(), caso.corpo(t))

			if decisao != DecisaoInvalida {
				t.Fatalf("esperava %q, obtive %q", DecisaoInvalida, decisao)
			}
			if len(c.permissoes.consultas) != 0 || len(c.executor.executadas) != 0 {
				t.Error("corpo inválido não pode chegar ao GitHub")
			}
			if tentativa := c.unicaTentativa(t); tentativa.Decisao != DecisaoInvalida || tentativa.Motivo != MotivoCorpoInvalido {
				t.Errorf("o pedido inválido autenticado também é registrado: %+v", tentativa)
			}
		})
	}
}

func TestServicoAceitaSolicitacaoDeAlteracaoComJustificativa(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{
		"tipo":          "solicitar_alteracao",
		"justificativa": "  falta teste para o caso de timeout  ",
	}))

	if decisao != DecisaoAceita {
		t.Fatalf("solicitação com justificativa precisa ser aceita, obtive %q", decisao)
	}
	if justificativa := c.executor.executadas[0].Justificativa; justificativa != "falta teste para o caso de timeout" {
		t.Errorf("a justificativa chega ao executor sem espaços nas pontas, obtive %q", justificativa)
	}
}

func TestServicoAceitaRepositorioComPontoHifenESublinhado(t *testing.T) {
	c := novoCenario(t)

	decisao := c.servico.Processar(t.Context(), corpoDoPedido(t, map[string]any{"repositorio": "org-x/meu_repo.go"}))

	if decisao != DecisaoAceita {
		t.Errorf("nome de repositório legítimo foi rejeitado: %q", decisao)
	}
}

func comCampos(campos map[string]any) func(t *testing.T) []byte {
	return func(t *testing.T) []byte {
		return corpoDoPedido(t, campos)
	}
}
