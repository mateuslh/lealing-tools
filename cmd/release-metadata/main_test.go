package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePublicaTodasAsToolsComChecksums(t *testing.T) {
	root := t.TempDir()
	manifests := filepath.Join(root, "manifests")
	dist := filepath.Join(root, "dist")
	if err := os.MkdirAll(manifests, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: lealing.dev/v1
id: demo
version: 1.2.3
name: Demo
summary: Demonstra uma tool.
detail: Teste.
category: utilities
risk: safe
glyph: "#"
runtime: {protocol: {min: 1, max: 1}}
platforms: [darwin-arm64, windows-amd64]
permissions: {filesystem: {read: [], write: []}, network: false, subprocess: false}
`
	if err := os.WriteFile(filepath.Join(manifests, "demo.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"demo_darwin_arm64.tar.gz", "demo_windows_amd64.zip"} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(manifests, dist, "v1.2.3", "mateuslh/lealing-tools", "mateuslh", "official", "0.3.1"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dist, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got index
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 1 || len(got.Tools[0].Artifacts) != 2 || got.Tools[0].Version != "1.2.3" {
		t.Fatalf("index = %+v", got)
	}
	checksums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil || !strings.Contains(string(checksums), "demo_manifest.yaml") ||
		!strings.Contains(string(checksums), "demo_windows_amd64.zip") {
		t.Fatalf("checksums = %s, erro = %v", checksums, err)
	}
	if _, err := os.Stat(filepath.Join(dist, "entries", "demo--1.2.3.json")); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateRejeitaVersaoDiferenteDoManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.yaml"), []byte(
		"apiVersion: lealing.dev/v1\nid: demo\nversion: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, t.TempDir(), "v1.0.1", "mateuslh/lealing-tools", "mateuslh", "official", "0.3.1"); err == nil {
		t.Fatal("versão divergente foi aceita")
	}
}
