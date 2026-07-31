package marketplaceindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validEntry(id, version string) Entry {
	return Entry{
		ID: id, Version: version, Name: "Demo", Summary: "Demonstra uma tool.",
		Category: "utilities", Risk: "safe", Publisher: "community-author", Channel: "community",
		ManifestURL: "https://example.test/releases/v1/manifest.yaml", MinimumEngine: "0.3.0",
		Protocol: VersionRange{Min: 1, Max: 1}, Permissions: Permissions{
			Filesystem: FilesystemPermissions{Read: []string{"~/.demo"}, Write: []string{}},
		},
		Artifacts: []Artifact{{
			Platform: "darwin-arm64", URL: "https://example.test/releases/v1/demo.tar.gz",
			SHA256: strings.Repeat("a", 64),
		}},
	}
}

func TestBuildOrdenaEConsolidaEntradas(t *testing.T) {
	root, entries, publishers := setup(t)
	writeEntry(t, entries, "z.json", validEntry("zeta", "1.0.0"))
	writeEntry(t, entries, "a.json", validEntry("alpha", "2.0.0"))

	index, err := Build(entries, publishers)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Tools) != 2 || index.Tools[0].ID != "alpha" || index.APIVersion != APIVersion {
		t.Fatalf("index = %+v", index)
	}
	output := filepath.Join(root, "index.json")
	raw, _ := Canonical(index)
	if err := os.WriteFile(output, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Check(index, output); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejeitaDuplicata(t *testing.T) {
	_, entries, publishers := setup(t)
	entry := validEntry("demo", "1.0.0")
	writeEntry(t, entries, "one.json", entry)
	writeEntry(t, entries, "two.json", entry)
	if _, err := Build(entries, publishers); err == nil || !strings.Contains(err.Error(), "duplicada") {
		t.Fatalf("Build = %v", err)
	}
}

func TestCanalConfiavelExigePublisherAutorizado(t *testing.T) {
	entry := validEntry("demo", "1.0.0")
	entry.Channel = "official"
	if err := entry.Validate(Publishers{Official: []string{"mateuslh"}}); err == nil || !strings.Contains(err.Error(), "não pode") {
		t.Fatalf("Validate = %v", err)
	}
	entry.Publisher = "mateuslh"
	if err := entry.Validate(Publishers{Official: []string{"mateuslh"}}); err != nil {
		t.Fatal(err)
	}
}

func TestEntradaRejeitaChecksumEPermissaoInvalidos(t *testing.T) {
	entry := validEntry("demo", "1.0.0")
	entry.Artifacts[0].SHA256 = "abc"
	if err := entry.Validate(Publishers{}); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("checksum = %v", err)
	}
	entry = validEntry("demo", "1.0.0")
	entry.Permissions.Filesystem.Read = []string{""}
	if err := entry.Validate(Publishers{}); err == nil || !strings.Contains(err.Error(), "permissão") {
		t.Fatalf("permissão = %v", err)
	}
}

func TestSemVerRejeitaZeroInicialEOrdenaReleaseDepoisDeRC(t *testing.T) {
	entry := validEntry("demo", "1.0.0-01")
	if err := entry.Validate(Publishers{}); err == nil || !strings.Contains(err.Error(), "versão") {
		t.Fatalf("versão = %v", err)
	}
	if !versionLess("1.0.0-rc.2", "1.0.0") || versionLess("1.0.0", "1.0.0-rc.2") {
		t.Fatal("ordem SemVer entre pre-release e release estável está incorreta")
	}
}

func setup(t *testing.T) (root, entries, publishers string) {
	t.Helper()
	root = t.TempDir()
	entries = filepath.Join(root, "tools")
	if err := os.Mkdir(entries, 0o700); err != nil {
		t.Fatal(err)
	}
	publishers = filepath.Join(root, "publishers.json")
	if err := os.WriteFile(publishers, []byte(`{"official":["mateuslh"],"verified":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, entries, publishers
}

func writeEntry(t *testing.T, directory, name string, entry Entry) {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
