// Command release-metadata gera os metadados publicados junto aos artefatos.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var semver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type artifact struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
}

type entry struct {
	ID            string         `json:"id"`
	Version       string         `json:"version"`
	Publisher     string         `json:"publisher"`
	ManifestURL   string         `json:"manifestUrl"`
	Artifacts     []artifact     `json:"artifacts"`
	Protocol      map[string]int `json:"protocol"`
	MinimumEngine string         `json:"minimumEngine"`
	Channel       string         `json:"channel"`
}

type index struct {
	APIVersion string  `json:"apiVersion"`
	Tools      []entry `json:"tools"`
}

var releases = []struct {
	platform string
	filename string
}{
	{"darwin-amd64", "token-usage_darwin_amd64.tar.gz"},
	{"darwin-arm64", "token-usage_darwin_arm64.tar.gz"},
	{"windows-amd64", "token-usage_windows_amd64.zip"},
	{"windows-arm64", "token-usage_windows_arm64.zip"},
}

func main() {
	manifestPath := flag.String("manifest", "manifests/token-usage.yaml", "manifest fonte")
	manifestOutput := flag.String("manifest-output", "", "destino opcional do manifest versionado")
	directory := flag.String("dir", "", "diretório contendo os quatro artefatos")
	version := flag.String("version", "", "versão SemVer sem v")
	repository := flag.String("repository", "mateuslh/lealing-tools", "owner/repository")
	flag.Parse()
	if err := generate(*manifestPath, *manifestOutput, *directory, *version, *repository); err != nil {
		fmt.Fprintln(os.Stderr, "release-metadata:", err)
		os.Exit(1)
	}
}

func generate(manifestPath, manifestOutput, directory, version, repository string) error {
	version = strings.TrimPrefix(version, "v")
	if !semver.MatchString(version) {
		return fmt.Errorf("versão inválida: %q", version)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	manifest, err = withVersion(manifest, version)
	if err != nil {
		return err
	}
	if manifestOutput != "" {
		if err := os.WriteFile(manifestOutput, manifest, 0o644); err != nil {
			return err
		}
	}
	if directory == "" {
		return nil
	}
	if repository == "" || strings.ContainsAny(repository, " \t\r\n") {
		return errors.New("repositório inválido")
	}
	baseURL := "https://github.com/" + repository + "/releases/download/v" + version
	artifacts := make([]artifact, 0, len(releases))
	var checksums strings.Builder
	for _, release := range releases {
		content, err := os.ReadFile(filepath.Join(directory, release.filename))
		if err != nil {
			return fmt.Errorf("ler %s: %w", release.filename, err)
		}
		sum := sha256.Sum256(content)
		digest := hex.EncodeToString(sum[:])
		fmt.Fprintf(&checksums, "%s  %s\n", digest, release.filename)
		artifacts = append(artifacts, artifact{
			Platform: release.platform,
			URL:      baseURL + "/" + release.filename,
			SHA256:   digest,
		})
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.yaml"), manifest, 0o644); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index{
		APIVersion: "lealing.dev/marketplace/v1",
		Tools: []entry{{
			ID: "token-usage", Version: version, Publisher: "mateuslh",
			ManifestURL: baseURL + "/manifest.yaml", Artifacts: artifacts,
			Protocol: map[string]int{"min": 1, "max": 1}, MinimumEngine: "0.3.0", Channel: "official",
		}},
	}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(directory, "index.json"), data, 0o644)
}

func withVersion(manifest []byte, version string) ([]byte, error) {
	lines := strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n")
	found := false
	for index, line := range lines {
		if strings.HasPrefix(line, "version:") {
			lines[index] = "version: " + version
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("manifest sem version")
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}
