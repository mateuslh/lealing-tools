package ui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	coreaccount "github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
	accountui "github.com/mateuslh/lealing-tools/internal/claudeaccounts/ui"
	devadapter "github.com/mateuslh/lealing-tools/internal/devkit/adapter"
	coredev "github.com/mateuslh/lealing-tools/internal/devkit/domain"
	devui "github.com/mateuslh/lealing-tools/internal/devkit/ui"
	corepower "github.com/mateuslh/lealing-tools/internal/power/domain"
	powerui "github.com/mateuslh/lealing-tools/internal/power/ui"
	coresystem "github.com/mateuslh/lealing-tools/internal/systeminfo/domain"
	systemui "github.com/mateuslh/lealing-tools/internal/systeminfo/ui"
	"github.com/mateuslh/lealing-tools/internal/ui/theme"
	"github.com/mateuslh/lealing-tools/internal/ui/tui"
)

var frames = []tui.Frame{
	{Width: 200, Height: 60}, {Width: 150, Height: 42}, {Width: 120, Height: 36},
	{Width: 100, Height: 30}, {Width: 84, Height: 26}, {Width: 70, Height: 22},
	{Width: 50, Height: 16}, {Width: 34, Height: 12}, {Width: 26, Height: 8},
}

type inspector struct{}

func (inspector) Inspect(context.Context) (coresystem.Snapshot, error) {
	return coresystem.Snapshot{OSVersion: "Sistema 1.0", Chip: "CPU", Memory: "16 GB"}, nil
}

type powerManager struct{}

func (powerManager) Read(context.Context) (corepower.Settings, error) {
	return corepower.Settings{}, nil
}
func (powerManager) Apply(context.Context, corepower.Settings) error { return nil }
func (powerManager) PasswordlessEnabled(context.Context) bool        { return true }
func (powerManager) EnablePasswordless(context.Context) error        { return nil }
func (powerManager) DisablePasswordless(context.Context) error       { return nil }
func (powerManager) Defaults() corepower.Settings                    { return corepower.Settings{} }
func (powerManager) Features() corepower.Feature                     { return corepower.AllFeatures }

type switcher struct{}

func (switcher) State(context.Context) (coreaccount.State, error) { return coreaccount.State{}, nil }
func (switcher) Save(context.Context, string) (coreaccount.Profile, error) {
	return coreaccount.Profile{}, nil
}
func (switcher) SaveOverwriting(context.Context, string) (coreaccount.Profile, error) {
	return coreaccount.Profile{}, nil
}
func (switcher) Activate(context.Context, string) error            { return nil }
func (switcher) ActivateOverwriting(context.Context, string) error { return nil }
func (switcher) Forget(context.Context, string) error              { return nil }

func settle(model tui.Screen) tui.Screen {
	if command := model.Init(); command != nil {
		if message := command(); message != nil {
			model, _ = model.Update(message)
		}
	}
	return model
}

func models(t *testing.T) map[string]tui.Screen {
	t.Helper()
	deps := tui.Deps{Theme: theme.Default()}
	definition, _ := coredev.DefinitionFor(coredev.ToolJSON)
	return map[string]tui.Screen{
		"system-info":     settle(systemui.New(deps, inspector{}, nil)),
		"power-control":   settle(powerui.New(deps, powerManager{})),
		"claude-accounts": settle(accountui.New(deps, switcher{}, nil)),
		"json-lab":        devui.New(deps, devadapter.New(), definition),
	}
}

func TestNoveGeometrias(t *testing.T) {
	for name, model := range models(t) {
		for _, frame := range frames {
			t.Run(name, func(t *testing.T) {
				model, _ = model.Update(tea.WindowSizeMsg{Width: frame.Width, Height: frame.Height})
				lines := strings.Split(model.View(frame), "\n")
				if len(lines) > frame.Height {
					t.Fatalf("%dx%d: %d linhas", frame.Width, frame.Height, len(lines))
				}
				for row, line := range lines {
					if width := lipgloss.Width(line); width > frame.Width {
						t.Fatalf("%dx%d linha %d mede %d", frame.Width, frame.Height, row, width)
					}
				}
			})
		}
	}
}
