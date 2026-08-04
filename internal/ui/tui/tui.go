// Package tui adapta os nomes usados pelas telas migradas ao SDK screen-v1.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mateuslh/lealing-sdk/protocol"
	"github.com/mateuslh/lealing-sdk/screen"
	"github.com/mateuslh/lealing-tools/internal/ui/theme"
)

type Screen = screen.Model
type ScreenID = string
type Frame = protocol.Frame
type Hint = protocol.Hint

type Deps struct{ Theme *theme.Theme }

// Back solicita ao host a navegação que antes era uma mensagem interna da
// engine. A capability é declarada nos dois manifests.
func Back() tea.Msg { return screen.Request(protocol.CapabilityNavigationBack, nil)() }
