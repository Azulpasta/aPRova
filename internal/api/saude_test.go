package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func dependenciaQueDemora(nome string, duracao time.Duration) Dependencia {
	return Dependencia{Nome: nome, Verificar: func(ctx context.Context) error {
		select {
		case <-time.After(duracao):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
}

func executarProntidao(t *testing.T, dependencias ...Dependencia) (int, respostaProntidao) {
	t.Helper()

	gravador := httptest.NewRecorder()
	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil)

	NovoServidor(dependencias...).Rotas().ServeHTTP(gravador, requisicao)

	var corpo respostaProntidao
	if err := json.NewDecoder(gravador.Body).Decode(&corpo); err != nil {
		t.Fatalf("resposta não é JSON válido: %v", err)
	}
	return gravador.Code, corpo
}

func TestProntidaoNaoReprovaDependenciaSaudavelPorCausaDaLentidaoDeOutra(t *testing.T) {
	const quaseNoLimite = 1200 * time.Millisecond

	codigo, corpo := executarProntidao(t,
		dependenciaQueDemora("primeira", quaseNoLimite),
		dependenciaQueDemora("segunda", quaseNoLimite),
	)

	if codigo != http.StatusOK {
		t.Errorf("as duas dependências respondem dentro do limite; esperava 200, obtive %d", codigo)
	}
	if corpo.Dependencias["segunda"] != situacaoDisponivel {
		t.Errorf("a segunda dependência é saudável, mas foi reportada como %q", corpo.Dependencias["segunda"])
	}
}

func dependenciaQuebrada(nome string) Dependencia {
	return Dependencia{Nome: nome, Verificar: func(context.Context) error {
		return errors.New("fora do ar")
	}}
}

func dependenciaSaudavel(nome string) Dependencia {
	return Dependencia{Nome: nome, Verificar: func(context.Context) error {
		return nil
	}}
}

func TestSaudeRespondeOkMesmoComDependenciaFora(t *testing.T) {
	gravador := httptest.NewRecorder()
	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)

	NovoServidor(dependenciaQuebrada("postgres")).Rotas().ServeHTTP(gravador, requisicao)

	if gravador.Code != http.StatusOK {
		t.Errorf("liveness não consulta dependências; esperava 200, obtive %d", gravador.Code)
	}
}

func TestProntidaoRespondeOkComTodasDisponiveis(t *testing.T) {
	codigo, corpo := executarProntidao(t, dependenciaSaudavel("postgres"), dependenciaSaudavel("redis"))

	if codigo != http.StatusOK {
		t.Errorf("esperava 200, obtive %d", codigo)
	}
	if corpo.Status != situacaoDisponivel {
		t.Errorf("esperava status %q, obtive %q", situacaoDisponivel, corpo.Status)
	}
	for _, nome := range []string{"postgres", "redis"} {
		if corpo.Dependencias[nome] != situacaoDisponivel {
			t.Errorf("dependência %s deveria estar %q, obtive %q", nome, situacaoDisponivel, corpo.Dependencias[nome])
		}
	}
}

func TestProntidaoIdentificaQualDependenciaFalhou(t *testing.T) {
	codigo, corpo := executarProntidao(t, dependenciaSaudavel("postgres"), dependenciaQuebrada("redis"))

	if codigo != http.StatusServiceUnavailable {
		t.Errorf("esperava 503, obtive %d", codigo)
	}
	if corpo.Dependencias["postgres"] != situacaoDisponivel {
		t.Errorf("postgres está no ar e deveria constar como %q, obtive %q", situacaoDisponivel, corpo.Dependencias["postgres"])
	}
	if corpo.Dependencias["redis"] != situacaoIndisponivel {
		t.Errorf("redis está fora e deveria constar como %q, obtive %q", situacaoIndisponivel, corpo.Dependencias["redis"])
	}
}

func TestProntidaoDeclaraContentTypeJSON(t *testing.T) {
	gravador := httptest.NewRecorder()
	requisicao := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", nil)

	NovoServidor(dependenciaSaudavel("postgres")).Rotas().ServeHTTP(gravador, requisicao)

	const esperado = "application/json; charset=utf-8"
	if obtido := gravador.Header().Get("Content-Type"); obtido != esperado {
		t.Errorf("esperava Content-Type %q, obtive %q", esperado, obtido)
	}
}
