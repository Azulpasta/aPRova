package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Azulpasta/aPRova/internal/acaoremota"
	"github.com/Azulpasta/aPRova/internal/api"
	"github.com/Azulpasta/aPRova/internal/config"
	"github.com/Azulpasta/aPRova/internal/platform"
	"github.com/Azulpasta/aPRova/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	tempoLimiteDesligamento     = 10 * time.Second
	tempoLimiteLeituraCabecalho = 10 * time.Second
)

func main() {
	if err := executar(); err != nil {
		slog.Error("api encerrada com erro", "erro", err)
		os.Exit(1)
	}
}

func executar() error {
	configuracao, err := config.Carregar()
	if err != nil {
		return err
	}

	config.ConfigurarLogPadrao(configuracao)

	ctx, encerrarEscutaDeSinais := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer encerrarEscutaDeSinais()

	bancoDados, err := repository.ConectarPostgres(ctx, configuracao.URLBancoDados)
	if err != nil {
		return err
	}
	defer bancoDados.Close()

	clienteRedis, err := repository.ConectarRedis(ctx, configuracao.URLRedis)
	if err != nil {
		return err
	}
	defer func() {
		if err := clienteRedis.Close(); err != nil {
			slog.Error("falha ao fechar o cliente redis", "erro", err)
		}
	}()

	acoesRemotas, err := montarAcoesRemotas(configuracao)
	if err != nil {
		return err
	}

	servidorHTTP := &http.Server{
		Addr: configuracao.EnderecoHTTP(),
		Handler: api.NovoServidor(api.Opcoes{
			Dependencias:        dependencias(bancoDados, clienteRedis),
			SegredoWebhook:      []byte(configuracao.GitHubSegredoWebhook),
			Execucoes:           repository.NovoRegistroDeExecucoes(bancoDados),
			SegredoAcoesRemotas: []byte(configuracao.N8NSegredoAcoes),
			AcoesRemotas:        acoesRemotas,
		}).Rotas(),
		ReadHeaderTimeout: tempoLimiteLeituraCabecalho,
	}

	falhaDeEscuta := make(chan error, 1)
	go func() {
		slog.Info("api escutando", "endereco", servidorHTTP.Addr)
		if err := servidorHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			falhaDeEscuta <- err
		}
	}()

	select {
	case err := <-falhaDeEscuta:
		return err
	case <-ctx.Done():
		slog.Info("sinal de encerramento recebido, desligando a api")
	}

	contextoDesligamento, cancelar := context.WithTimeout(context.Background(), tempoLimiteDesligamento)
	defer cancelar()

	return servidorHTTP.Shutdown(contextoDesligamento)
}

func montarAcoesRemotas(configuracao config.Configuracao) (api.ProcessadorDeAcoesRemotas, error) {
	if configuracao.N8NSegredoAcoes == "" {
		slog.Warn("N8N_SEGREDO_ACOES ausente, /acoes-remotas recusará todo pedido")
		return nil, nil
	}

	autenticador, err := platform.NovoAutenticador(platform.OpcoesAutenticador{
		AppID:           configuracao.GitHubAppID,
		ChavePrivadaPEM: configuracao.GitHubChavePrivada,
	})
	if err != nil {
		return nil, fmt.Errorf("ações remotas exigem as credenciais do GitHub App: %w", err)
	}

	clienteGitHub := platform.NovoCliente(platform.OpcoesCliente{Tokens: autenticador})

	return acaoremota.NovoServico(acaoremota.OpcoesServico{
		Identidades: acaoremota.NovoResolvedorEmMemoria(configuracao.VinculosSlackGitHub),
		Permissoes:  clienteGitHub,
		Autoria:     clienteGitHub,
		Executor:    acaoremota.NovoExecutorNoGitHub(clienteGitHub, acaoremota.ReenfileiradorIndisponivel{}),
		Tentativas:  acaoremota.NovoRegistroEmMemoria(),
		Reservas:    acaoremota.NovaReservaEmMemoria(),
	})
}

func dependencias(bancoDados *pgxpool.Pool, clienteRedis *redis.Client) []api.Dependencia {
	return []api.Dependencia{
		{Nome: "postgres", Verificar: bancoDados.Ping},
		{Nome: "redis", Verificar: func(ctx context.Context) error {
			return clienteRedis.Ping(ctx).Err()
		}},
	}
}
