# lealing-tools

Tools oficiais instaláveis do [lealing](https://github.com/mateuslh/lealing).
Cada tool é um processo independente que conversa com a engine pelo protocolo
público `screen-v1`; este repositório não importa nenhum pacote `internal/` da
engine, nem o módulo da engine — o SDK (`protocol`, `screen`, `component`,
`machine`) vem de [`github.com/mateuslh/lealing-sdk`](https://github.com/mateuslh/lealing-sdk),
um repositório independente. A engine só entra em tempo de desenvolvimento,
como binário externo chamado via `go run` para validar manifest e instalar
localmente.

## Tools disponíveis

- `token-usage`: consumo, custos e cotas do Claude Code e Codex;
- `system-info`: sistema, chip, memória, uptime e bateria;
- `power-control`: perfis de energia no macOS e Windows;
- `claude-accounts`: alternância explícita de contas do Claude Code;
- `http-probe`, `network-inspector`, `json-lab`, `jwt-inspector`,
  `cidr-calculator`, `codec-lab`, `checksum-lab` e `uuid-generator`:
  bancada de engenharia dividida em executáveis instaláveis independentes.

Nenhuma delas é compilada dentro da engine.

## Desenvolvimento

Requer Go 1.26.5 ou posterior.

```sh
make fmt
make vet
make test
make race
make cross
make manifest
```

O executável reserva stdout exclusivamente ao protocolo e envia logs para
stderr. I/O de arquivo, rede e subprocesso é executado de forma assíncrona pelo
model, usando somente caminhos e permissões recebidos da engine.

Para começar uma tool pública, copie o
[`examples/hello-tool`](examples/hello-tool). O exemplo compila no mesmo módulo,
usa apenas o SDK público e inclui model, hints, tema, teste de geometria e
manifest mínimos. `token-usage` é a referência vertical para tabs, gráficos,
scroll e I/O assíncrono real.

## Instalação local

```sh
make build VERSION=1.0.0
lealing -tool-install ./bin/token-usage
lealing -tool-install ./bin/system-info
# repita para os demais diretórios em bin/
lealing -tools
```

Depois que a versão estiver no índice público, a instalação não exige clone
nem download manual:

```sh
lealing -marketplace
lealing -tool-install token-usage
lealing -tool-update token-usage
```

Atualização e rollback continuam sendo operações da engine:

```sh
lealing -tool-update ./bin/token-usage
lealing -tool-rollback token-usage
lealing -tool-disable token-usage
lealing -tool-enable token-usage
lealing -tool-remove token-usage
```

## Artefatos oficiais

Cada tool só entra no marketplace depois que seus pacotes para Darwin e
Windows em amd64/arm64, manifest e checksums forem publicados. A
[última release](https://github.com/mateuslh/lealing-tools/releases/latest)
e o [índice consolidado](marketplace/index.json) são as fontes do que já foi
publicado; uma pasta gerada por `make build` continua instalável localmente
antes disso.

O índice consolidado usado pela engine fica em
[`marketplace/index.json`](marketplace/index.json). Autores de tools podem
publicar artefatos em seus próprios repositórios e enviar uma entrada pelo
[guia do marketplace](marketplace/README.md); nenhuma implementação precisa
ser movida para este repositório.

Uma publicação só ocorre depois de fmt, vet, testes, race, validação do
manifest, conformidade do protocolo e toda a matriz de cross-build ficarem
verdes. O workflow `official-tools` é um chamador fino do pipeline reusável
[`publish-tools.yml`](https://github.com/mateuslh/lealing-sdk/blob/main/.github/workflows/publish-tools.yml)
do `lealing-sdk`, que também é usado por `lealing-tools-bradesco` e `bravars` —
a lógica de build/empacotamento/release fica num só lugar.
