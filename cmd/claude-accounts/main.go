package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	adapter "github.com/mateuslh/lealing-tools/internal/claudeaccounts/adapter"
	core "github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
	ui "github.com/mateuslh/lealing-tools/internal/claudeaccounts/ui"
	"github.com/mateuslh/lealing-tools/internal/ui/theme"
	uitui "github.com/mateuslh/lealing-tools/internal/ui/tui"
	"github.com/mateuslh/lealing/sdk/protocol"
	"github.com/mateuslh/lealing/sdk/screen"
)

var version = "dev"

var modelFactory screen.Factory = func(session screen.Session) screen.Model {
	home, _ := os.UserHomeDir()
	credentials := adapter.CredentialsPath(home)
	manager := core.NewManager(
		adapter.NewVault(credentials),
		adapter.NewConfigFile(adapter.ConfigPath(home), filepath.Join(session.Initialize.DataDir, "claude-json.backup")),
		adapter.NewStore(filepath.Join(session.Initialize.DataDir, "claude-accounts.json")),
		nil,
	).WithSettings(adapter.NewSettingsFile(adapter.SettingsPath(home)))
	return ui.New(uitui.Deps{Theme: theme.From(session.Initialize.Theme)}, manager, nil)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runProtocol(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "claude-accounts:", err)
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
