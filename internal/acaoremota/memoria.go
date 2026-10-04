package acaoremota

import (
	"context"
	"maps"
	"sync"
)

// ResolvedorEmMemoria resolve logins a partir de um mapa fixo, carregado da
// configuração. Atende apenas desenvolvimento e testes.
type ResolvedorEmMemoria struct {
	loginPorUsuario map[string]string
}

// NovoResolvedorEmMemoria constrói o resolvedor a partir do mapa de usuário
// do Slack para login do GitHub.
func NovoResolvedorEmMemoria(loginPorUsuario map[string]string) *ResolvedorEmMemoria {
	return &ResolvedorEmMemoria{loginPorUsuario: maps.Clone(loginPorUsuario)}
}

// LoginGitHub devolve o login vinculado ao usuário do Slack, ou ErrSemVinculo
// quando não há vínculo.
func (r *ResolvedorEmMemoria) LoginGitHub(_ context.Context, usuarioSlack string) (string, error) {
	login, vinculado := r.loginPorUsuario[usuarioSlack]
	if !vinculado {
		return "", ErrSemVinculo
	}
	return login, nil
}

// RegistroEmMemoria guarda as tentativas na memória do processo.
type RegistroEmMemoria struct {
	mutex      sync.Mutex
	tentativas []Tentativa
}

// NovoRegistroEmMemoria constrói um registro vazio.
func NovoRegistroEmMemoria() *RegistroEmMemoria {
	return &RegistroEmMemoria{}
}

// Registrar guarda a tentativa.
func (r *RegistroEmMemoria) Registrar(_ context.Context, tentativa Tentativa) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.tentativas = append(r.tentativas, tentativa)
	return nil
}

// Tentativas devolve uma cópia das tentativas registradas, na ordem em que
// chegaram.
func (r *RegistroEmMemoria) Tentativas() []Tentativa {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return append([]Tentativa(nil), r.tentativas...)
}

// ReservaEmMemoria guarda os id_acao reservados na memória do processo. Não
// sobrevive a reinício nem cruza processos.
type ReservaEmMemoria struct {
	mutex      sync.Mutex
	reservados map[string]struct{}
}

// NovaReservaEmMemoria constrói uma reserva vazia.
func NovaReservaEmMemoria() *ReservaEmMemoria {
	return &ReservaEmMemoria{reservados: map[string]struct{}{}}
}

// Reservar marca o id_acao como usado, devolvendo false quando ele já tinha
// sido reservado.
func (r *ReservaEmMemoria) Reservar(_ context.Context, idAcao string) (bool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, jaReservado := r.reservados[idAcao]; jaReservado {
		return false, nil
	}

	r.reservados[idAcao] = struct{}{}
	return true, nil
}
