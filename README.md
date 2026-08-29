# aPRova

O aPRova é um GitHub App escrito em Go que funciona como camada de confiabilidade
sobre agentes de IA que escrevem código. Ele recebe o webhook de um Pull Request,
executa a mudança em ambiente isolado, classifica o risco resultante, publica um
recibo assinado com a decisão e condiciona o merge à aprovação humana quando o
risco é alto. Esta é a fundação do projeto: a API sobe, responde às rotas de
saúde e aceita o webhook, mas nenhuma dessas regras está implementada ainda.

## Requisitos

- Go 1.25 ou superior
- Docker e Docker Compose
- golangci-lint, para o alvo `make lint`

## Subindo localmente

Copie o arquivo de exemplo e ajuste o que for necessário. Os valores que já vêm
preenchidos funcionam contra o Compose deste repositório.

```sh
cp .env.example .env
```

Suba o PostgreSQL e o Redis e espere ficarem saudáveis:

```sh
make up
```

Aplique as migrações:

```sh
make migrate
```

Em um terminal, suba a API:

```sh
make run-api
```

Em outro, suba o worker:

```sh
make run-worker
```

Para derrubar os serviços:

```sh
make down
```

## Rotas

| Método | Rota               | Descrição                                              |
| ------ | ------------------ | ------------------------------------------------------ |
| GET    | `/health`          | Responde 200 se o processo está no ar                  |
| GET    | `/ready`           | Responde 200 se PostgreSQL e Redis respondem, 503 se não |
| POST   | `/webhooks/github` | Aceita o webhook com 202 e ainda não o processa        |

```sh
curl -i localhost:8080/health
curl -i localhost:8080/ready
```

## Testes

Testes unitários ficam no mesmo pacote do código testado, como arquivos
`*_test.go`. O diretório `tests/` é reservado para testes que cruzam pacotes ou
dependem de serviço externo: `tests/integration` (Postgres e Redis reais),
`tests/functional` (requisitos de negócio ponta a ponta) e `tests/e2e` (contra
Pull Request real).

```sh
make test
make lint
```

## Geração de código

As queries em `queries/` e o schema em `migrations/` alimentam o sqlc, que gera
o código de acesso a dados em `internal/repository`:

```sh
sqlc generate
```

## Licença

MIT. Veja [LICENSE](LICENSE).
