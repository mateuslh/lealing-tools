# Contrato para tools oficiais do lealing

Este repositório contém somente tools externas. A engine vive em
`github.com/mateuslh/lealing`; nenhuma tool pode importar pacotes
`github.com/mateuslh/lealing/internal/...`.

Antes de editar:

1. rode `git status --short`;
2. rode `make test`;
3. preserve alterações existentes;
4. leia por inteiro a tool alterada e seu manifest.

## Estrutura

```text
cmd/<tool>/                 executável e composição
internal/<tool>/            domínio, adapters e model da tool
manifests/<tool>.yaml       descoberta e permissões
sdk importado da engine     protocol, screen e component
```

O ID do manifest é permanente. `summary` tem uma linha e termina com ponto.
O executable é um nome simples, sem argumentos ou caminho. Declare somente as
plataformas, requisitos, capabilities e permissões realmente usados.

`Update` e `View` não fazem I/O. Arquivo, HTTP, subprocesso e espera rodam em
`tea.Cmd` com contexto e timeout. stdout pertence exclusivamente ao framing
`Content-Length`; logs vão para stderr.

O Body pode usar Unicode e SGR de cor, bold, italic, underline e reset. Não
emita OSC, clipboard, título, clear-screen, cursor global, alt-screen ou
mudança de modo. A engine sanitiza, mas a tool não deve produzir sequências
proibidas.

Antes de concluir, rode nesta ordem:

```sh
make fmt
make vet
make test
make race
make cross
make manifest
```

Uma release é solicitada pelo workflow `official-tools`, com uma versão nova.
Nunca mova ou recrie uma tag existente. A pipeline valida tudo, cria a tag no
commit remoto e publica binários, manifest, checksums e índice somente depois
da matriz verde.
