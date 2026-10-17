# Imagem do sandbox Go

Imagem que roda dentro do job do sandbox para repositórios Go. O único processo
do aPRova aqui dentro é o executor de dentro (`cmd/sandbox`), que recebe o
pedido por variáveis de ambiente, clona o repositório no SHA exato, roda
`go mod download`, `go build ./...`, compila os testes e roda
`go test -json ./...`, e entrega um `ResultadoSandbox` como uma única linha
JSON no stdout.

## Construir

O contexto é a raiz do repositório, porque o primeiro estágio compila
`cmd/sandbox`. O `.dockerignore` da raiz libera apenas `go.mod`, `go.sum`,
`cmd/` e `internal/`; `.env` e chaves nunca entram no contexto.

```sh
make imagem-sandbox-go
```

A base é `golang:1.27.1-alpine`, fixada por digest. Para atualizar, troque a
tag e o digest juntos nos dois `FROM` e rode os testes de integração.

Na Azure, a imagem é publicada pelo workflow `imagem-sandbox-go` com a tag do
SHA do commit, nunca `latest`, e o worker a escolhe por `SANDBOX_IMAGEM_GO`.

Tamanho na versão inicial: 406 MB descomprimida, cerca de 78 MB comprimida. A
base sozinha responde por 373 MB, porque o toolchain do Go precisa estar
presente para compilar e testar o código do PR.

## Variáveis de entrada

| Variável | Conteúdo |
| --- | --- |
| `APROVA_REPOSITORIO` | URL `https` (ou `file` nos testes), sem credencial embutida |
| `APROVA_SHA` | SHA completo, 40 ou 64 caracteres hexadecimais |
| `APROVA_LINGUAGEM` | `go` |
| `APROVA_TASK_RUN_ID` | identificador da execução |
| `APROVA_PRAZO_SEGUNDOS` | prazo total do executor de dentro |
| `APROVA_SAIDA` | `stdout` (padrão) ou `coletor` |

Qualquer variável ausente ou inválida vira resultado `infraestrutura` com a
mensagem que nomeia a variável. O processo termina com código 0 sempre que
entrega um resultado, seja ele qual for, e com 1 apenas quando nem a entrega
foi possível.

## Por que o código do PR não forja o resultado

O código do PR é não confiável e roda aqui dentro. Se ele rodasse com o mesmo
usuário do executor, poderia escrever no stdout do executor por
`/proc/<pid>/fd/1` e publicar um "passou" falso.

- O executor de dentro roda como root, sem `USER` na imagem, mas o executor
  local sobe o container com `--cap-drop ALL --cap-add SETUID --cap-add SETGID`:
  ele só consegue trocar de usuário.
- Todo comando que toca o código do PR (`git`, `go mod download`, `go build`,
  `go test`) roda como uid 10001, via `SysProcAttr.Credential`, sem grupos
  suplementares. Um processo do uid 10001 não abre `/proc/1/fd/1`, que
  pertence ao root.
- O clone pertence ao uid 10001 e o executor não escreve nada nele. A única
  leitura é conferir se existe `go.mod` na raiz, antes de qualquer código do
  PR rodar. O resultado é montado só a partir do que chega pelos pipes, nunca
  de arquivo do diretório de trabalho.
- A saída dos comandos é capturada por pipe. O que o PR imprime vira texto
  dentro de `saida_bruta`, nunca uma linha própria no stdout do container.
- Ao fim de cada comando, todo processo do uid 10001 que ficou para trás é
  encerrado com `kill -KILL -1` disparado pelo próprio uid 10001, porque o
  executor não tem `CAP_KILL`. Um processo em segundo plano segurando o pipe
  não prende o executor: a leitura desiste após `WaitDelay`.

O teste de integração `TestSandboxGoTesteMaliciosoNaoForjaOResultado` tenta
exatamente esses ataques e confere que o resultado entregue é a falha real.

## Limites conhecidos

- **O código do PR tem acesso de rede durante os testes.** Não há segredo
  nenhum no container, nenhum token do GitHub App e nenhuma credencial no
  clone, então o dano possível é limitado a usar a rede a partir do job, mas
  ele existe. A restrição de rede é de outro card.
- Um PR pode inflar `testes_passaram` imprimindo linhas que imitam o
  enquadramento do `test2json`. Ele não consegue esconder um teste que falhou
  nem fazer o `go test` sair com sucesso, porque a falha real continua no
  fluxo e o pacote continua reprovado.
- Um PR pode encerrar o próprio `go test`, que roda com o mesmo uid 10001.
  Sem evento de falha, o resultado vira `infraestrutura`, nunca "passou".
- O executor local derruba as capacidades; no Azure Container Apps a
  separação depende do uid 10001, que é o que impede a forja. Confirmar quais
  capacidades o Container Apps concede ao container é pendência do card de
  infraestrutura.
