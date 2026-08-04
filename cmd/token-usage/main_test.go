package main

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/mateuslh/lealing-sdk/protocol"
)

func TestGrantedReadUsaSomentePathEntreguePelaEngine(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".codex", "sessions")
	initialize := protocol.Initialize{Permissions: protocol.Permissions{
		Filesystem: protocol.FilesystemPermissions{Read: []string{root}},
	}}
	if got := grantedRead(initialize, ".codex/sessions"); got != root {
		t.Fatalf("grant = %q", got)
	}
	if got := grantedRead(initialize, ".claude/projects"); got != "" {
		t.Fatalf("path não concedido = %q", got)
	}
}

func TestConformidadeScreenV1(t *testing.T) {
	toolConn, engineConn := net.Pipe()
	defer engineConn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := runProtocol(ctx, toolConn, toolConn)
		_ = toolConn.Close()
		done <- err
	}()

	encoder := protocol.NewEncoder(engineConn)
	decoder := protocol.NewDecoder(engineConn)
	initialize := protocol.Initialize{
		Protocol: protocol.VersionRange{Min: protocol.Version1, Max: protocol.Version1},
		ToolID:   "token-usage",
		Frame:    protocol.Frame{Width: 80, Height: 24},
	}
	message, err := protocol.NewMessage(protocol.Version1, 1, protocol.MethodInitialize, initialize)
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.Write(message); err != nil {
		t.Fatal(err)
	}
	initializedMessage, err := decoder.Read()
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := protocol.DecodePayload[protocol.Initialized](initializedMessage)
	if err != nil {
		t.Fatal(err)
	}
	if initializedMessage.Method != protocol.MethodInitialized || initialized.UIMode != protocol.UIModeScreenV1 || initialized.State != "ready" {
		t.Fatalf("initialized = %+v", initialized)
	}

	shutdown, err := protocol.NewMessage(protocol.Version1, 2, protocol.MethodShutdown, protocol.Shutdown{Reason: "teste"})
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.Write(shutdown); err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, engineConn) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("tool não encerrou após shutdown")
	}
}
