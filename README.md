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

## Configuração

Copie o arquivo de exemplo e ajuste o que for necessário. Os valores que já vêm
preenchidos funcionam contra o Compose deste repositório.

```sh
cp .env.example .env
```

O `.env` é ignorado pelo Git e nunca deve ser commitado. Nesta etapa apenas
`DATABASE_URL` e `REDIS_URL` são obrigatórias; as demais são opcionais e passam
a ser exigidas conforme cada funcionalidade for implementada. Subir sem uma
variável obrigatória falha na inicialização, com uma mensagem que nomeia a
variável e diz onde obter o valor.

`SANDBOX_EXECUTOR` aceita `local`, `cloudrun` ou `actions`. Hoje só `local`
precisa funcionar.

### Chaves de assinatura dos recibos

Gere o par Ed25519 e cole as duas linhas no `.env`:

```sh
make keys
```

A chave privada é segredo e fica só no `.env`. **A chave pública precisa ser
publicada** — neste README, num endpoint da aplicação ou em qualquer lugar
estável e acessível.

Isso não é detalhe operacional: é o que define a garantia do projeto. Com HMAC,
verificar um recibo exige o mesmo segredo usado para assiná-lo, então só quem
emitiu consegue conferir — e "confie em nós" volta a ser a única resposta
possível. Com assinatura Ed25519 a chave pública basta para verificar e não
serve para forjar, então qualquer pessoa (auditoria, cliente, o próprio autor do
PR) confere um recibo sozinha, sem pedir acesso a nada. É essa verificação
independente que transforma o recibo em prova, em vez de registro interno.

As duas chaves são decodificadas e validadas na inicialização, não no primeiro
uso: uma chave malformada derruba o processo no deploy, em vez de falhar no
momento em que houvesse um recibo real para assinar.

## Subindo localmente

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
