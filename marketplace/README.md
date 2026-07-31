# Publicando no marketplace

O índice público é aberto a tools `screen-v1` de qualquer repositório. A
engine lê somente [`index.json`](index.json); esse arquivo é gerado de forma
determinística a partir de uma entrada por versão em [`tools/`](tools/). A
descoberta e a indexação nunca baixam nem executam o binário.

## Como enviar uma tool

1. Publique uma release imutável com `manifest.yaml` e pacotes `.tar.gz`
   (Darwin) ou `.zip` (Windows). Cada pacote contém apenas o manifest e o
   executável correspondente à plataforma.
2. Calcule o SHA-256 do pacote completo, não somente do executável.
3. Copie `tools/token-usage--1.0.0.json` para
   `tools/<publisher>--<id>--<versão>.json` e preencha os metadados. O ID deve
   ser o mesmo ID permanente do manifest.
4. Comece com `channel: community`. Os canais `verified` e `official` exigem
   que o publicador esteja explicitamente listado em `publishers.json`.
5. Rode `go run ./cmd/marketplace-index` e depois `make marketplace`.
6. Abra um pull request incluindo a entrada e o índice consolidado.

O review confere identidade do publicador, manifest, permissões, risco,
checksums, compatibilidade e proveniência dos artefatos. Um checksum protege o
download contra alteração acidental ou comprometimento isolado do hosting; ele
não substitui revisão do publicador. A v1 ainda não possui assinatura
criptográfica do índice nem sandbox de sistema operacional.

## Atualizações

Não altere uma entrada já publicada. Adicione outro arquivo com a nova SemVer,
URLs imutáveis e checksums novos. A engine escolhe a versão mais recente que
intersecta o protocolo suportado, atende `minimumEngine` e contém um artefato
para a plataforma corrente. Rollback continua usando a versão anterior mantida
na instalação local.

## Validação

```sh
go run ./cmd/marketplace-index -check
go test ./internal/marketplaceindex
```

O contrato também está descrito por [`schema.json`](schema.json). A validação
do gerador é autoritativa e adicionalmente impede duplicatas e uso indevido dos
canais confiáveis.
