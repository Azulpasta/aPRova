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

A base é `golang:1.27.2-alpine`, fixada por digest. Para atualizar, troque a
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
| `APROVA_DIRETORIO` | opcional: caminho do projeto dentro do repositório; ausente ou vazio é a raiz. Vem da configuração do aPRova, nunca do PR |

Qualquer variável obrigatória ausente, ou qualquer variável inválida, vira
resultado `infraestrutura` com a mensagem que nomeia a variável. O diretório do
projeto é conferido depois do clone: se ele não existe, não é diretório ou passa
por um link simbólico em qualquer componente do caminho, o resultado também é
`infraestrutura` e nada é testado. Link é recusado mesmo apontando para dentro do
clone, porque faria o sandbox testar outro projeto que não o configurado. O processo termina com código 0 sempre que
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
- O clone pertence ao uid 10001 e o executor não escreve nada nele. As únicas
  leituras acontecem antes de qualquer código do PR rodar: quando há
  `APROVA_DIRETORIO`, conferir com `lstat` cada componente desse caminho; e
  conferir se existe `go.mod` na raiz do projeto, que é o próprio clone quando
  `APROVA_DIRETORIO` está ausente. O resultado é montado só a partir do que
  chega pelos pipes, nunca de arquivo do diretório de trabalho.
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
- O executor local derruba as capacidades; no Azure Container Apps isso não é
  possível, e a separação depende só do uid 10001. Ver a seção abaixo.

## O que foi observado no Azure Container Apps

Medido em 2026-10-17, no job `aprova-sandbox-teste` (0,25 vCPU, 0,5 GiB), com a
imagem `aprova-sandbox-go:cf7ae14e5fd302449b405cd04e31b67b50c0bc40`.

**Capacidades.** O template do job não tem campo para capacidades nem para o
usuário de execução, e o `az containerapp job create` não oferece opção para
isso. Uma execução de diagnóstico com a mesma imagem leu `/proc/self/status` do
processo inicial:

| Campo | Valor |
| --- | --- |
| `Uid` | `0` |
| `CapEff` | `00000000a80425fb`: CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, SETGID, SETUID, SETPCAP, NET_BIND_SERVICE, NET_RAW, SYS_CHROOT, MKNOD, AUDIT_WRITE, SETFCAP |
| `CapAmb` | `0000000000000000` |
| `NoNewPrivs` | `1` |

Ou seja, na Azure o executor de dentro roda como root com o conjunto padrão de
capacidades de container, sem poder derrubá-las, e o próprio ambiente liga o
`no_new_privs`. A troca para o uid 10001 funcionou nos dois disparos reais. Ao
trocar de root para um uid comum, o processo perde as capacidades, o conjunto
ambiente está vazio e o `no_new_privs` impede recuperá-las por binário com
setuid. O código do PR, portanto, roda sem capacidade nenhuma, e não consegue
abrir `/proc/1/fd/1` do root. Se a troca algum dia for negada, o resultado é
`infraestrutura` com a mensagem "não foi possível rodar os testes como usuário
sem privilégio", e nada do PR roda.

O que se perde em relação ao executor local é a defesa em profundidade do lado
do root: com `DAC_OVERRIDE` e `KILL`, um defeito no executor de dentro teria
mais alcance na Azure do que no Docker local.

**Tempo por execução**, contra o repositório-alvo `aPRova-TESTES` com
`APROVA_DIRETORIO=go`:

| SHA | Resultado | `duracao_ms` do executor de dentro | Do disparo ao `Succeeded` visto pelo script |
| --- | --- | --- | --- |
| `2ce4ba4` (main) | nenhuma, 10 passaram, 0 falharam | 37668 (37,7 s) | 75 s |
| `d7ee920` (teste-peso-zero) | teste, 10 passaram, 1 falhou | 38724 (38,7 s) | 78 s |

A última coluna é uma medida inclusiva: contém o `duracao_ms`, a Azure criando
a execução, baixando a imagem e subindo o container, e a granularidade de 5 s
com que `scripts/disparar-sandbox-azure` consulta o status. Os 0,5 GiB bastaram para compilar a stdlib e os
testes. A linha JSON do resultado chega ao Log Analytics (tabela
`ContainerAppConsoleLogs_CL`, `Stream_s == "stdout"`) em menos de um minuto
depois do fim.
