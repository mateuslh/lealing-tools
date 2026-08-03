// Command release-metadata gera metadados de uma release com várias tools.
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
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
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
	Name          string         `json:"name"`
	Summary       string         `json:"summary"`
	Detail        string         `json:"detail,omitempty"`
	Category      string         `json:"category"`
	Risk          string         `json:"risk"`
	Glyph         string         `json:"glyph,omitempty"`
	Permissions   permissions    `json:"permissions"`
	Publisher     string         `json:"publisher"`
	ManifestURL   string         `json:"manifestUrl"`
	Artifacts     []artifact     `json:"artifacts"`
	Protocol      map[string]int `json:"protocol"`
	MinimumEngine string         `json:"minimumEngine"`
	Channel       string         `json:"channel"`
}

type permissions struct {
	Filesystem struct {
		Read  []string `json:"read" yaml:"read"`
		Write []string `json:"write" yaml:"write"`
	} `json:"filesystem" yaml:"filesystem"`
	Network    bool `json:"network" yaml:"network"`
	Subprocess bool `json:"subprocess" yaml:"subprocess"`
}

type index struct {
	APIVersion string  `json:"apiVersion"`
	Tools      []entry `json:"tools"`
}

type manifest struct {
	APIVersion  string      `yaml:"apiVersion"`
	ID          string      `yaml:"id"`
	Version     string      `yaml:"version"`
	Name        string      `yaml:"name"`
	Summary     string      `yaml:"summary"`
	Detail      string      `yaml:"detail"`
	Category    string      `yaml:"category"`
	Risk        string      `yaml:"risk"`
	Glyph       string      `yaml:"glyph"`
	Platforms   []string    `yaml:"platforms"`
	Permissions permissions `yaml:"permissions"`
	Runtime     struct {
		Protocol struct {
			Min int `yaml:"min"`
			Max int `yaml:"max"`
		} `yaml:"protocol"`
	} `yaml:"runtime"`
}

func main() {
	manifests := flag.String("manifests", "manifests", "diretório de manifests fonte")
	directory := flag.String("dir", "dist", "diretório contendo os artefatos")
	version := flag.String("version", "", "versão da release e das tools (vX.Y.Z)")
	repository := flag.String("repository", "mateuslh/lealing-tools", "owner/repository")
	publisher := flag.String("publisher", "mateuslh", "publisher gravado no índice")
	channel := flag.String("channel", "official", "canal do marketplace")
	minimumEngine := flag.String("minimum-engine", "0.3.1", "engine mínima")
	flag.Parse()
	if err := generate(*manifests, *directory, *version, *repository, *publisher, *channel, *minimumEngine); err != nil {
		fmt.Fprintln(os.Stderr, "release-metadata:", err)
		os.Exit(1)
	}
}

func generate(manifestsDir, directory, version, repository, publisher, channel, minimumEngine string) error {
	version = strings.TrimPrefix(version, "v")
	if !semver.MatchString(version) {
		return fmt.Errorf("versão inválida: %q", version)
	}
	if repository == "" || strings.ContainsAny(repository, " \t\r\n") || strings.Count(repository, "/") != 1 {
		return errors.New("repositório inválido")
	}
	if strings.TrimSpace(publisher) == "" || strings.TrimSpace(channel) == "" || !semver.MatchString(minimumEngine) {
		return errors.New("política de publicação inválida")
	}

	paths, err := filepath.Glob(filepath.Join(manifestsDir, "*.yaml"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return errors.New("nenhum manifest encontrado")
	}
	if err := os.MkdirAll(filepath.Join(directory, "entries"), 0o755); err != nil {
		return err
	}

	baseURL := "https://github.com/" + repository + "/releases/download/v" + version
	result := index{APIVersion: "lealing.dev/marketplace/v1"}
	checksums := map[string]string{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var source manifest
		if err := yaml.Unmarshal(raw, &source); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if source.APIVersion != "lealing.dev/v1" || source.ID == "" || source.Version != version {
			return fmt.Errorf("%s precisa declarar apiVersion lealing.dev/v1 e version %s", path, version)
		}
		manifestName := source.ID + "_manifest.yaml"
		if err := os.WriteFile(filepath.Join(directory, manifestName), raw, 0o644); err != nil {
			return err
		}
		checksums[manifestName] = digest(raw)

		artifacts := make([]artifact, 0, len(source.Platforms))
		platforms := append([]string(nil), source.Platforms...)
		sort.Strings(platforms)
		for _, platform := range platforms {
			filename, err := artifactName(source.ID, platform)
			if err != nil {
				return err
			}
			content, err := os.ReadFile(filepath.Join(directory, filename))
			if err != nil {
				return fmt.Errorf("ler %s: %w", filename, err)
			}
			checksum := digest(content)
			checksums[filename] = checksum
			artifacts = append(artifacts, artifact{
				Platform: platform, URL: baseURL + "/" + filename, SHA256: checksum,
			})
		}
		item := entry{
			ID: source.ID, Version: source.Version, Name: source.Name,
			Summary: source.Summary, Detail: source.Detail, Category: source.Category,
			Risk: source.Risk, Glyph: source.Glyph, Permissions: source.Permissions,
			Publisher: publisher, ManifestURL: baseURL + "/" + manifestName,
			Artifacts:     artifacts,
			Protocol:      map[string]int{"min": source.Runtime.Protocol.Min, "max": source.Runtime.Protocol.Max},
			MinimumEngine: minimumEngine, Channel: channel,
		}
		result.Tools = append(result.Tools, item)
		entryRaw, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			return err
		}
		entryRaw = append(entryRaw, '\n')
		entryName := source.ID + "--" + source.Version + ".json"
		if err := os.WriteFile(filepath.Join(directory, "entries", entryName), entryRaw, 0o644); err != nil {
			return err
		}
	}

	indexRaw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	indexRaw = append(indexRaw, '\n')
	if err := os.WriteFile(filepath.Join(directory, "index.json"), indexRaw, 0o644); err != nil {
		return err
	}

	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	var lines strings.Builder
	for _, name := range names {
		fmt.Fprintf(&lines, "%s  %s\n", checksums[name], name)
	}
	return os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(lines.String()), 0o644)
}

func artifactName(id, platform string) (string, error) {
	osName, arch, ok := strings.Cut(platform, "-")
	if !ok || (osName != "darwin" && osName != "windows") || (arch != "amd64" && arch != "arm64") {
		return "", fmt.Errorf("plataforma inválida em %s: %s", id, platform)
	}
	extension := ".tar.gz"
	if osName == "windows" {
		extension = ".zip"
	}
	return id + "_" + osName + "_" + arch + extension, nil
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
