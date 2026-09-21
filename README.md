# aPRova

O aPRova é um GitHub App escrito em Go que funciona como camada de confiabilidade
sobre agentes de IA que escrevem código. Ele recebe o webhook de um Pull Request,
executa a mudança em ambiente isolado, classifica o risco resultante, publica um
recibo assinado com a decisão e condiciona o merge à aprovação humana quando o
risco é alto. Esta é a fundação do projeto: a API sobe, responde às rotas de
saúde e aceita o webhook, mas nenhuma dessas regras está implementada ainda.

## Requisitos

- Go 1.25.14 ou superior (versão fixada na diretiva `toolchain` do `go.mod`)
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

## Integração contínua

O pipeline está em [.github/workflows/ci.yml](.github/workflows/ci.yml) e roda a
cada push em qualquer branch e em todo pull request para `main`.

As etapas rodam em sequência, das mais baratas para as mais caras, de modo que
uma falha trivial não espere a suíte inteira:

| Ordem | Etapa | O que barra |
| --- | --- | --- |
| 1 | `gitleaks` | credencial no código ou no histórico |
| 2 | `go vet` | erro que compila mas está errado |
| 3 | `golangci-lint` | erro ignorado, problema de segurança, complexidade |
| 4 | `go build` | quebra de compilação |
| 5 | `go test -race` | teste reprovado e condição de corrida |
| 6 | `govulncheck` | CVE alcançável nas dependências |

O gitleaks vem primeiro porque é a falha mais grave e a mais rápida de detectar,
e roda contra o histórico completo (`--log-opts="--all --full-history"`), não só
contra os arquivos atuais: um segredo removido num commit posterior continua
exposto em quem já clonou.

Falha em qualquer etapa reprova o build. O relatório de cobertura é publicado
como artefato da execução, sob o nome `cobertura`.

As versões das ferramentas estão fixadas no bloco `env` do workflow. A versão do
Go não está lá: ela vem da diretiva `toolchain` do `go.mod`, que o `setup-go` lê
através de `go-version-file`. Fonte única, para que workflow e projeto não
possam divergir.

### Reproduzindo o CI localmente

A diretiva `toolchain` é um **mínimo**: se o seu Go local for mais novo, ele não
faz downgrade, e você acaba verificando contra uma biblioteca padrão diferente
da que o CI usa. Isso já causou um falso verde aqui — o pipeline reprovou com 28
vulnerabilidades da stdlib que localmente não apareciam.

Para rodar contra exatamente o mesmo toolchain do CI:

```sh
GOTOOLCHAIN=go1.25.14 go test ./... -race
GOTOOLCHAIN=go1.25.14 govulncheck ./...
```

Ao subir a versão do Go, altere a diretiva `toolchain` no `go.mod` e refaça a
verificação acima. Fixar um patch antigo é o mesmo que abrir mão das correções
de segurança publicadas depois dele.

### Proteção da branch principal

Isto não é configurável por código: é ajuste no próprio GitHub, em
**Settings → Branches → Branch protection rules**, feito por quem administra o
repositório.

Na regra para `main`, marque como obrigatório:

- **Require status checks to pass before merging** e, na busca de checks,
  selecione **`verificacao`** — é o nome do job do workflow, e cobre as seis
  etapas acima de uma vez
- **Require branches to be up to date before merging**, para que o check tenha
  rodado contra o estado real do merge
- **Do not allow bypassing the above settings**, senão a proteção vira sugestão

O check só aparece na lista depois que o workflow rodar ao menos uma vez na
branch. Se a lista estiver vazia, faça um push qualquer e volte à tela.

## Licença

MIT. Veja [LICENSE](LICENSE).
