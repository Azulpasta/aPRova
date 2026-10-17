package processo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

// ErrTrocaDeUsuarioNegada indica que o sistema recusou rodar o comando com a
// credencial sem privilégio. O comando não rodou e não é repetido sem ela.
var ErrTrocaDeUsuarioNegada = errors.New("processo: não foi possível rodar os testes como usuário sem privilégio")

const (
	esperaAposEncerrarPadrao = 5 * time.Second
	tempoLimiteDaLimpeza     = 10 * time.Second
)

// Credencial identifica o usuário e o grupo com que os comandos rodam.
type Credencial struct {
	UID uint32
	GID uint32
}

// Opcoes configura o rodador. Sem Credencial, os comandos rodam com o usuário
// do processo atual. EsperaAposEncerrar limita quanto tempo a leitura da saída
// espera depois que o processo principal termina ou é encerrado.
type Opcoes struct {
	Credencial         *Credencial
	EsperaAposEncerrar time.Duration
}

// RodadorDoSistema executa comandos do sistema operacional, capturando a saída
// por pipe e encerrando, ao fim de cada comando, todo processo que ele deixou
// para trás.
type RodadorDoSistema struct {
	credencial         *Credencial
	esperaAposEncerrar time.Duration
	iniciar            func(*exec.Cmd) error
}

// NovoRodador constrói o rodador com as opções informadas.
func NovoRodador(opcoes Opcoes) *RodadorDoSistema {
	espera := opcoes.EsperaAposEncerrar
	if espera <= 0 {
		espera = esperaAposEncerrarPadrao
	}
	return &RodadorDoSistema{credencial: opcoes.Credencial, esperaAposEncerrar: espera, iniciar: (*exec.Cmd).Start}
}

// Rodar executa o comando com exatamente o ambiente informado e devolve stdout
// e stderr juntos. Prazo do contexto esgotado encerra o processo e todos os
// seus descendentes.
func (r *RodadorDoSistema) Rodar(ctx context.Context, comando Comando) Execucao {
	var saida bytes.Buffer

	processo := exec.CommandContext(ctx, comando.Binario, comando.Argumentos...)
	processo.Dir = comando.Diretorio
	processo.Env = append([]string{}, comando.Ambiente...)
	processo.Stdout = &saida
	processo.Stderr = &saida
	processo.SysProcAttr = r.atributosDoProcesso()
	processo.WaitDelay = r.esperaAposEncerrar
	processo.Cancel = func() error { return r.encerrarDescendentes(processo) }

	if err := r.iniciador()(processo); err != nil {
		return interpretar(ctx, nil, r.explicarFalhaAoIniciar(err))
	}

	err := processo.Wait()
	r.limparRestantes(processo)

	return interpretar(ctx, saida.Bytes(), err)
}

func (r *RodadorDoSistema) iniciador() func(*exec.Cmd) error {
	if r.iniciar == nil {
		return (*exec.Cmd).Start
	}
	return r.iniciar
}

func (r *RodadorDoSistema) explicarFalhaAoIniciar(err error) error {
	if r.credencial != nil && errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("%w (uid %d): %w", ErrTrocaDeUsuarioNegada, r.credencial.UID, err)
	}
	return err
}

func (r *RodadorDoSistema) atributosDoProcesso() *syscall.SysProcAttr {
	atributos := &syscall.SysProcAttr{Setpgid: true}
	if r.credencial != nil {
		atributos.Credential = &syscall.Credential{Uid: r.credencial.UID, Gid: r.credencial.GID, Groups: []uint32{}}
	}
	return atributos
}

func (r *RodadorDoSistema) encerrarDescendentes(processo *exec.Cmd) error {
	if r.credencial == nil {
		return syscall.Kill(-processo.Process.Pid, syscall.SIGKILL)
	}
	return r.encerrarTodosDoUsuario()
}

func (r *RodadorDoSistema) limparRestantes(processo *exec.Cmd) {
	err := r.encerrarDescendentes(processo)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		slog.Warn("falha ao encerrar processos remanescentes", "erro", err)
	}
}

func (r *RodadorDoSistema) encerrarTodosDoUsuario() error {
	ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteDaLimpeza)
	defer cancelar()

	encerrador := exec.CommandContext(ctx, "kill", "-KILL", "-1")
	encerrador.Env = []string{}
	encerrador.SysProcAttr = r.atributosDoProcesso()

	var codigo *exec.ExitError
	if err := encerrador.Run(); err != nil && !errors.As(err, &codigo) {
		return err
	}
	return nil
}

func interpretar(ctx context.Context, saida []byte, err error) Execucao {
	execucao := Execucao{Saida: saida}

	var saiuComCodigo *exec.ExitError
	switch {
	case ctx.Err() != nil:
		execucao.PrazoEsgotado = true
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
	case errors.As(err, &saiuComCodigo):
		execucao.CodigoSaida = saiuComCodigo.ExitCode()
	default:
		execucao.Erro = err
	}

	return execucao
}
