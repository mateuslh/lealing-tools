package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/mateuslh/lealing-sdk/protocol"
	"github.com/mateuslh/lealing-sdk/screen"
	"github.com/mateuslh/lealing-tools/internal/power/macos"
	ui "github.com/mateuslh/lealing-tools/internal/power/ui"
	"github.com/mateuslh/lealing-tools/internal/power/windows"
	"github.com/mateuslh/lealing-tools/internal/ui/theme"
	uitui "github.com/mateuslh/lealing-tools/internal/ui/tui"
)

var version = "dev"

var modelFactory screen.Factory = func(session screen.Session) screen.Model {
	deps := uitui.Deps{Theme: theme.From(session.Initialize.Theme)}
	if session.Initialize.Platform == "windows" {
		return ui.New(deps, windows.NewPowerManager())
	}
	return ui.New(deps, macos.NewPowerManager())
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runProtocol(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "power-control:", err)
		os.Exit(1)
	}
}

func runProtocol(ctx context.Context, input io.Reader, output io.Writer) error {
	return screen.Run(ctx, screen.Config{
		ToolVersion: version, Protocol: protocol.VersionRange{Min: 1, Max: 1},
		Input: input, Output: output,
		Factory: modelFactory,
	})
}
