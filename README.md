# lealing-tools

Tools oficiais instaláveis do [lealing](https://github.com/mateuslh/lealing).
Cada tool é um processo independente que conversa com a engine pelo protocolo
público `screen-v1`; este repositório não importa nenhum pacote `internal/` da
engine.

## Tool disponível

- `token-usage`: agrega consumo, custos e cotas do Claude Code e Codex. Mantém
  permanentemente o ID `token-usage`.

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

## Instalação local

```sh
make build VERSION=1.0.0
lealing -tool-install ./bin
lealing -tools
```

Atualização e rollback continuam sendo operações da engine:

```sh
lealing -tool-update ./bin
lealing -tool-rollback token-usage
lealing -tool-remove token-usage
```

## Artefatos oficiais

A [última release](https://github.com/mateuslh/lealing-tools/releases/latest)
contém binários para Darwin e Windows em amd64/arm64, `manifest.yaml`,
`checksums.txt` e `index.json`. O índice estável pode ser obtido pela
[URL da última versão](https://github.com/mateuslh/lealing-tools/releases/latest/download/index.json).

Uma publicação só ocorre depois de fmt, vet, testes, race, validação do
manifest, conformidade do protocolo e toda a matriz de cross-build ficarem
verdes.
