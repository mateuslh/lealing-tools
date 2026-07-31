# Hello Tool

Copie este diretório para iniciar uma tool. Troque permanentemente `id`, nome
do executável e metadados, implemente seu domínio fora do `main` e mantenha
todo I/O em `tea.Cmd`. O exemplo importa somente `sdk/protocol`, `sdk/screen`
e `sdk/component`; nenhum pacote `internal/` da engine é público.

Depois do primeiro release estável do SDK, fixe uma versão SemVer compatível:

```sh
go get github.com/mateuslh/lealing@v0.3.0
```

Para testar localmente, compile o binário com o nome declarado, copie
`manifest.yaml` para o mesmo diretório e rode:

```sh
lealing -tool-validate ./pacote
lealing -tool-install ./pacote
```

Quando a release tiver pacotes e checksums para as plataformas declaradas,
siga `marketplace/README.md` para enviar a entrada comunitária.
