package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePublicaManifestChecksumsEIndice(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "source.yaml")
	if err := os.WriteFile(manifest, []byte("apiVersion: lealing.dev/v1\nid: token-usage\nversion: 0.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, release := range releases {
		if err := os.WriteFile(filepath.Join(dir, release.filename), []byte(release.platform), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(manifest, "", dir, "v1.2.3", "mateuslh/lealing-tools"); err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(filepath.Join(dir, "manifest.yaml"))
	if err != nil || !strings.Contains(string(published), "version: 1.2.3") {
		t.Fatalf("manifest = %q, erro = %v", published, err)
	}
	checksums, err := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("darwin-amd64"))
	if !strings.Contains(string(checksums), hex.EncodeToString(want[:])+"  token-usage_darwin_amd64.tar.gz") {
		t.Fatalf("checksums = %s", checksums)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got index
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 1 || len(got.Tools[0].Artifacts) != 4 || got.Tools[0].Version != "1.2.3" || got.Tools[0].Name == "" || len(got.Tools[0].Permissions.Filesystem.Read) != 4 {
		t.Fatalf("index = %+v", got)
	}
}

func TestGenerateRejeitaVersaoInvalida(t *testing.T) {
	if err := generate("inexistente", "", "", "1.2", "mateuslh/lealing-tools"); err == nil {
		t.Fatal("versão inválida aceita")
	}
}
