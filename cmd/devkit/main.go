package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	adapter "github.com/mateuslh/lealing-tools/internal/devkit/adapter"
	core "github.com/mateuslh/lealing-tools/internal/devkit/domain"
	ui "github.com/mateuslh/lealing-tools/internal/devkit/ui"
	"github.com/mateuslh/lealing-tools/internal/ui/theme"
	uitui "github.com/mateuslh/lealing-tools/internal/ui/tui"
	"github.com/mateuslh/lealing/sdk/protocol"
	"github.com/mateuslh/lealing/sdk/screen"
)

var version = "dev"

var modelFactory screen.Factory = func(session screen.Session) screen.Model {
	definition, ok := definitionForID(session.Initialize.ToolID)
	if !ok {
		definition = core.Definitions()[0]
	}
	return ui.New(uitui.Deps{Theme: theme.From(session.Initialize.Theme)}, adapter.New(), definition)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runProtocol(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devkit:", err)
		os.Exit(1)
	}
}

func runProtocol(ctx context.Context, input io.Reader, output io.Writer) error {
	return screen.Run(ctx, screen.Config{
		ToolVersion: version, Protocol: protocol.VersionRange{Min: 1, Max: 1},
		Capabilities: []string{protocol.CapabilityNavigationBack},
		Input:        input, Output: output,
		Factory: modelFactory,
	})
}

func definitionForID(id string) (core.Definition, bool) {
	for _, definition := range core.Definitions() {
		if definition.ToolID == id {
			return definition, true
		}
	}
	return core.Definition{}, false
}
