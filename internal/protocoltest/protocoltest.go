// Package protocoltest exercita o handshake mínimo de executáveis screen-v1.
package protocoltest

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mateuslh/lealing/sdk/protocol"
	"github.com/mateuslh/lealing/sdk/screen"
)

// Runner é a fronteira testável do main de cada executável.
type Runner func(context.Context, io.Reader, io.Writer) error

// Factory evita que o teste de framing dispare I/O específico da tool.
func Factory(screen.Session) screen.Model { return model{} }

type model struct{}

func (model) Init() tea.Cmd                            { return nil }
func (m model) Update(tea.Msg) (screen.Model, tea.Cmd) { return m, nil }
func (model) View(protocol.Frame) string               { return "tool pronta" }

// Check valida initialize, negociação screen-v1 e encerramento limpo.
func Check(toolID string, run Runner) error {
	toolConn, engineConn := net.Pipe()
	defer engineConn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := run(ctx, toolConn, toolConn)
		_ = toolConn.Close()
		done <- err
	}()

	encoder := protocol.NewEncoder(engineConn)
	decoder := protocol.NewDecoder(engineConn)
	initialize := protocol.Initialize{
		Protocol: protocol.VersionRange{Min: protocol.Version1, Max: protocol.Version1},
		ToolID:   toolID, Frame: protocol.Frame{Width: 80, Height: 24},
	}
	message, err := protocol.NewMessage(protocol.Version1, 1, protocol.MethodInitialize, initialize)
	if err != nil {
		return err
	}
	if err := encoder.Write(message); err != nil {
		return err
	}
	initializedMessage, err := decoder.Read()
	if err != nil {
		return err
	}
	initialized, err := protocol.DecodePayload[protocol.Initialized](initializedMessage)
	if err != nil {
		return err
	}
	if initializedMessage.Method != protocol.MethodInitialized ||
		initialized.UIMode != protocol.UIModeScreenV1 || initialized.State != "ready" {
		return fmt.Errorf("initialized inválido: método=%s payload=%+v", initializedMessage.Method, initialized)
	}

	shutdown, err := protocol.NewMessage(protocol.Version1, 2, protocol.MethodShutdown, protocol.Shutdown{Reason: "teste"})
	if err != nil {
		return err
	}
	if err := encoder.Write(shutdown); err != nil {
		return err
	}
	go func() { _, _ = io.Copy(io.Discard, engineConn) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errorsForTimeout(ctx)
	}
}

func errorsForTimeout(ctx context.Context) error {
	return fmt.Errorf("tool não encerrou após shutdown: %w", ctx.Err())
}
