// Command marketplace-index consolida contribuições no índice consumido pela
// engine. Ele valida somente metadados; nenhum binário é baixado ou executado.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mateuslh/lealing-tools/internal/marketplaceindex"
)

func main() {
	entries := flag.String("entries", "marketplace/tools", "diretório com uma entrada JSON por versão")
	publishers := flag.String("publishers", "marketplace/publishers.json", "política de canais por publicador")
	output := flag.String("output", "marketplace/index.json", "índice consolidado")
	check := flag.Bool("check", false, "falha se o índice consolidado estiver desatualizado")
	flag.Parse()

	index, err := marketplaceindex.Build(*entries, *publishers)
	if err == nil && *check {
		err = marketplaceindex.Check(index, *output)
	} else if err == nil {
		var raw []byte
		raw, err = marketplaceindex.Canonical(index)
		if err == nil {
			err = os.WriteFile(*output, raw, 0o644)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "marketplace-index:", err)
		os.Exit(1)
	}
}
