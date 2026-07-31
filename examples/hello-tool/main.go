// Command hello-tool é o menor exemplo executável de uma tool screen-v1.
package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mateuslh/lealing/sdk/component"
	"github.com/mateuslh/lealing/sdk/protocol"
	"github.com/mateuslh/lealing/sdk/screen"
)

var version = "dev"

type model struct {
	theme *component.Theme
	count int
}

func (*model) Init() tea.Cmd { return nil }

func (m *model) Update(message tea.Msg) (screen.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k", "+":
			m.count++
		case "down", "j", "-":
			m.count--
		}
	}
	return m, nil
}

func (m *model) View(frame protocol.Frame) string {
	content := component.Center(max(frame.Width-2, 1), max(frame.Height-2, 1),
		m.theme.Title.Render("Olá, screen-v1"),
		m.theme.Body.Render(fmt.Sprintf("contador: %d", m.count)),
		m.theme.Dim.Render("use ↑ e ↓"),
	)
	return component.Panel{
		Title: "hello tool", Glyph: "✦", Accent: m.theme.Primary,
		Focused: true, Width: frame.Width, Height: frame.Height,
	}.Render(m.theme, content)
}

func (*model) Hints() []protocol.Hint {
	return []protocol.Hint{{Key: "↑↓", Label: "alterar"}, {Key: "esc", Label: "voltar"}}
}

func (m *model) Status() string { return fmt.Sprintf("contador %d", m.count) }

func main() {
	err := screen.Run(context.Background(), screen.Config{
		ToolVersion: version,
		Protocol:    protocol.VersionRange{Min: protocol.Version1, Max: protocol.Version1},
		Factory: func(session screen.Session) screen.Model {
			return &model{theme: component.ThemeFrom(session.Initialize.Theme)}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello-tool:", err)
		os.Exit(1)
	}
}
