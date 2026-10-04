package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/platform"
)

const tempoLimiteConsulta = 30 * time.Second

type consulta struct {
	instalacaoID int64
	dono         string
	repositorio  string
	login        string
}

func main() {
	if err := executar(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}

func executar() error {
	pedido, err := lerArgumentos(os.Args)
	if err != nil {
		return err
	}

	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}

	autenticador, err := platform.NovoAutenticador(platform.OpcoesAutenticador{
		AppID:           configuracao.GitHubAppID,
		ChavePrivadaPEM: configuracao.GitHubChavePrivada,
	})
	if err != nil {
		return err
	}

	ctx, cancelar := context.WithTimeout(context.Background(), tempoLimiteConsulta)
	defer cancelar()

	permissao, err := platform.NovoCliente(platform.OpcoesCliente{Tokens: autenticador}).
		PermissaoNoRepositorio(ctx, pedido.instalacaoID, pedido.dono, pedido.repositorio, pedido.login)
	if err != nil {
		return explicarFalha(err)
	}

	fmt.Println(descreverPermissao(pedido, permissao))
	return nil
}

func lerArgumentos(argumentos []string) (consulta, error) {
	uso := fmt.Errorf("uso: %s <installation_id> <dono/repositorio> <login_github>", argumentos[0])
	if len(argumentos) != 4 {
		return consulta{}, uso
	}

	instalacaoID, err := strconv.ParseInt(argumentos[1], 10, 64)
	if err != nil {
		return consulta{}, fmt.Errorf("installation_id precisa ser numérico, recebi %q", argumentos[1])
	}

	dono, repositorio, separado := strings.Cut(argumentos[2], "/")
	if !separado || dono == "" || repositorio == "" {
		return consulta{}, fmt.Errorf("repositório precisa estar no formato dono/nome, recebi %q", argumentos[2])
	}

	login := strings.TrimSpace(argumentos[3])
	if login == "" {
		return consulta{}, uso
	}

	return consulta{instalacaoID: instalacaoID, dono: dono, repositorio: repositorio, login: login}, nil
}

func descreverPermissao(pedido consulta, permissao platform.Permissao) string {
	veredito := acaoremota.NovaPolitica().Avaliar(acaoremota.Situacao{
		Tipo:      acaoremota.TipoAprovar,
		Permissao: permissao,
		Login:     pedido.login,
	})

	conclusao := "não pode aprovar"
	if veredito.Autorizada {
		conclusao = "pode aprovar PR de outra pessoa"
	}

	return fmt.Sprintf("%s tem papel %s em %s/%s: %s pelo aPRova",
		pedido.login, permissao, pedido.dono, pedido.repositorio, conclusao)
}

func explicarFalha(err error) error {
	switch {
	case errors.Is(err, platform.ErrNaoEncontrado):
		return fmt.Errorf("%w\nconfira se o login existe e se o installation_id cobre este repositório", err)
	case errors.Is(err, platform.ErrConfiguracaoInvalida):
		return fmt.Errorf("%w\nconfira GITHUB_APP_ID e GITHUB_PRIVATE_KEY_BASE64 no .env", err)
	case errors.Is(err, platform.ErrAppNaoInstalado):
		return fmt.Errorf("%w\ninstale o app no repositório e use o installation_id do evento", err)
	default:
		return err
	}
}
