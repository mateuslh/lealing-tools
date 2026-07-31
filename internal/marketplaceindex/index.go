// Package marketplaceindex valida e consolida as contribuições públicas do
// índice. Ele usa somente a biblioteca padrão e nunca abre ou executa os
// artefatos declarados.
package marketplaceindex

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const APIVersion = "lealing.dev/marketplace/v1"

var validID = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var validVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

type VersionRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type Artifact struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
}

type FilesystemPermissions struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

type Permissions struct {
	Filesystem FilesystemPermissions `json:"filesystem"`
	Network    bool                  `json:"network"`
	Subprocess bool                  `json:"subprocess"`
}

type Entry struct {
	ID            string       `json:"id"`
	Version       string       `json:"version"`
	Name          string       `json:"name"`
	Summary       string       `json:"summary"`
	Detail        string       `json:"detail,omitempty"`
	Category      string       `json:"category"`
	Risk          string       `json:"risk"`
	Glyph         string       `json:"glyph,omitempty"`
	Permissions   Permissions  `json:"permissions"`
	Publisher     string       `json:"publisher"`
	ManifestURL   string       `json:"manifestUrl"`
	Artifacts     []Artifact   `json:"artifacts"`
	Protocol      VersionRange `json:"protocol"`
	MinimumEngine string       `json:"minimumEngine"`
	Channel       string       `json:"channel"`
}

type Index struct {
	APIVersion string  `json:"apiVersion"`
	Tools      []Entry `json:"tools"`
}

type Publishers struct {
	Official []string `json:"official"`
	Verified []string `json:"verified"`
}

func Build(entriesDirectory, publishersPath string) (Index, error) {
	policy, err := loadPublishers(publishersPath)
	if err != nil {
		return Index{}, err
	}
	entries, err := os.ReadDir(entriesDirectory)
	if err != nil {
		return Index{}, err
	}
	index := Index{APIVersion: APIVersion}
	for _, file := range entries {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		entry, err := readEntry(filepath.Join(entriesDirectory, file.Name()))
		if err != nil {
			return Index{}, fmt.Errorf("%s: %w", file.Name(), err)
		}
		if err := entry.Validate(policy); err != nil {
			return Index{}, fmt.Errorf("%s: %w", file.Name(), err)
		}
		index.Tools = append(index.Tools, entry)
	}
	if len(index.Tools) == 0 {
		return Index{}, errors.New("nenhuma entrada encontrada")
	}
	sort.Slice(index.Tools, func(i, j int) bool {
		if index.Tools[i].ID != index.Tools[j].ID {
			return index.Tools[i].ID < index.Tools[j].ID
		}
		return versionLess(index.Tools[i].Version, index.Tools[j].Version)
	})
	for position := 1; position < len(index.Tools); position++ {
		previous, current := index.Tools[position-1], index.Tools[position]
		if previous.ID == current.ID && previous.Version == current.Version {
			return Index{}, fmt.Errorf("entrada duplicada: %s@%s", current.ID, current.Version)
		}
	}
	return index, nil
}

func Canonical(index Index) ([]byte, error) {
	raw, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func Check(index Index, outputPath string) error {
	want, err := Canonical(index)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s está desatualizado; rode go run ./cmd/marketplace-index", outputPath)
	}
	return nil
}

func (e Entry) Validate(policy Publishers) error {
	switch {
	case !validID.MatchString(e.ID):
		return fmt.Errorf("ID inválido: %q", e.ID)
	case !validSemver(e.Version):
		return fmt.Errorf("versão inválida: %q", e.Version)
	case strings.TrimSpace(e.Name) == "":
		return errors.New("name é obrigatório")
	case strings.ContainsAny(e.Summary, "\r\n") || !strings.HasSuffix(strings.TrimSpace(e.Summary), "."):
		return errors.New("summary deve ter uma linha e terminar com ponto")
	case !knownCategory(e.Category):
		return fmt.Errorf("categoria desconhecida: %q", e.Category)
	case e.Risk != "safe" && e.Risk != "caution" && e.Risk != "destructive":
		return fmt.Errorf("risk desconhecido: %q", e.Risk)
	case strings.TrimSpace(e.Publisher) == "":
		return errors.New("publisher é obrigatório")
	case e.Channel != "official" && e.Channel != "verified" && e.Channel != "community":
		return fmt.Errorf("channel desconhecido: %q", e.Channel)
	case e.Channel == "official" && !contains(policy.Official, e.Publisher):
		return fmt.Errorf("publisher %q não pode usar o canal official", e.Publisher)
	case e.Channel == "verified" && !contains(policy.Verified, e.Publisher):
		return fmt.Errorf("publisher %q não pode usar o canal verified", e.Publisher)
	case e.Protocol.Min <= 0 || e.Protocol.Max < e.Protocol.Min:
		return errors.New("intervalo de protocolo inválido")
	case e.MinimumEngine != "" && !validSemver(e.MinimumEngine):
		return fmt.Errorf("minimumEngine inválida: %q", e.MinimumEngine)
	case !safeHTTPS(e.ManifestURL):
		return errors.New("manifestUrl precisa usar HTTPS")
	case len(e.Artifacts) == 0:
		return errors.New("artifacts não pode ser vazio")
	}
	seen := map[string]bool{}
	for _, artifact := range e.Artifacts {
		switch {
		case !knownPlatform(artifact.Platform):
			return fmt.Errorf("plataforma desconhecida: %q", artifact.Platform)
		case seen[artifact.Platform]:
			return fmt.Errorf("plataforma duplicada: %s", artifact.Platform)
		case !safeHTTPS(artifact.URL):
			return fmt.Errorf("URL de %s precisa usar HTTPS", artifact.Platform)
		case !validSHA256(artifact.SHA256):
			return fmt.Errorf("SHA-256 inválido em %s", artifact.Platform)
		}
		seen[artifact.Platform] = true
	}
	return validatePermissions(e.Permissions)
}

func readEntry(path string) (Entry, error) {
	file, err := os.Open(path)
	if err != nil {
		return Entry{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var entry Entry
	if err := decoder.Decode(&entry); err != nil {
		return Entry{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Entry{}, errors.New("conteúdo adicional depois da entrada")
	}
	return entry, nil
}

func loadPublishers(path string) (Publishers, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Publishers{}, err
	}
	var policy Publishers
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return Publishers{}, err
	}
	return policy, nil
}

func validatePermissions(permissions Permissions) error {
	seen := map[string]bool{}
	for kind, paths := range map[string][]string{
		"filesystem.read":  permissions.Filesystem.Read,
		"filesystem.write": permissions.Filesystem.Write,
	} {
		for _, value := range paths {
			value = strings.TrimSpace(value)
			if value == "" || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("permissão %s inválida", kind)
			}
			key := kind + "\x00" + value
			if seen[key] {
				return fmt.Errorf("permissão %s duplicada: %s", kind, value)
			}
			seen[key] = true
		}
	}
	return nil
}

func knownCategory(value string) bool {
	switch value {
	case "system", "ai", "network", "media", "dev", "utilities":
		return true
	default:
		return false
	}
}

func knownPlatform(value string) bool {
	switch value {
	case "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64":
		return true
	default:
		return false
	}
}

func safeHTTPS(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validSemver(value string) bool {
	_, ok := parseVersion(value)
	return ok
}

type semanticVersion struct {
	core       [3]int
	prerelease []string
}

func parseVersion(value string) (semanticVersion, bool) {
	value = strings.TrimPrefix(value, "v")
	if !validVersion.MatchString(value) {
		return semanticVersion{}, false
	}
	core, prerelease, hasPrerelease := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	var result semanticVersion
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, false
		}
		result.core[index] = number
	}
	if hasPrerelease {
		result.prerelease = strings.Split(prerelease, ".")
		for _, identifier := range result.prerelease {
			if _, err := strconv.Atoi(identifier); err == nil && len(identifier) > 1 && identifier[0] == '0' {
				return semanticVersion{}, false
			}
		}
	}
	return result, true
}

func versionLess(left, right string) bool {
	a, aOK := parseVersion(left)
	b, bOK := parseVersion(right)
	if !aOK || !bOK {
		return left < right
	}
	for index := range a.core {
		if a.core[index] != b.core[index] {
			return a.core[index] < b.core[index]
		}
	}
	if len(a.prerelease) == 0 || len(b.prerelease) == 0 {
		return len(a.prerelease) > 0 && len(b.prerelease) == 0
	}
	for index := 0; index < min(len(a.prerelease), len(b.prerelease)); index++ {
		left, right := a.prerelease[index], b.prerelease[index]
		if left == right {
			continue
		}
		leftNumber, leftErr := strconv.Atoi(left)
		rightNumber, rightErr := strconv.Atoi(right)
		switch {
		case leftErr == nil && rightErr == nil:
			return leftNumber < rightNumber
		case leftErr == nil:
			return true
		case rightErr == nil:
			return false
		default:
			return left < right
		}
	}
	return len(a.prerelease) < len(b.prerelease)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
