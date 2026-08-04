package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/mateuslh/lealing-sdk/component"
	"github.com/mateuslh/lealing-sdk/protocol"
)

func TestExemploRespeitaGeometria(t *testing.T) {
	for _, frame := range []protocol.Frame{
		{Width: 150, Height: 42}, {Width: 60, Height: 20}, {Width: 26, Height: 8},
	} {
		model := &model{theme: component.DefaultTheme()}
		for index, line := range strings.Split(model.View(frame), "\n") {
			if width := lipgloss.Width(line); width > frame.Width {
				t.Errorf("%dx%d linha %d tem %d colunas", frame.Width, frame.Height, index, width)
			}
		}
	}
}
