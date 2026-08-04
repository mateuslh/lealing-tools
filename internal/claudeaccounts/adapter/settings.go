package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
	"github.com/mateuslh/lealing-tools/internal/support/xdg"
)

const settingsName = "settings.json"

const (
	oauthTokenEnv = "CLAUDE_CODE_OAUTH_TOKEN"
	apiKeyEnv     = "ANTHROPIC_API_KEY"
	authTokenEnv  = "ANTHROPIC_AUTH_TOKEN"
)

var managedAuthEnv = []string{authTokenEnv, apiKeyEnv, oauthTokenEnv}

// SettingsPath devolve o arquivo de preferências de usuário da CLI.
func SettingsPath(home string) string {
	return filepath.Join(home, claudeDir, settingsName)
}

// SettingsFile é o ~/.claude/settings.json visto como um todo.
//
// O perfil guarda o arquivo inteiro porque a configuração de uma conta não
// cabe numa lista de chaves conhecidas: junto do token vêm o endpoint do
// gateway, os nomes de modelo que só aquele gateway entende e o que mais o
// usuário precisou para aquela conta funcionar. Trocar só o token deixava
// tudo isso apontado para o lugar da conta anterior.
type SettingsFile struct {
	Path   string
	getenv func(string) (string, bool)
}

var _ ccaccount.Settings = (*SettingsFile)(nil)

func NewSettingsFile(path string) *SettingsFile {
	return &SettingsFile{Path: path, getenv: os.LookupEnv}
}

// Snapshot implementa ccaccount.Settings.
//
// Devolve o arquivo como está, sem reformatar: o que sai daqui é o que vai
// para o perfil, e comparar snapshots é como a tool percebe que a conta ativa
// mudou de configuração.
func (a *SettingsFile) Snapshot(context.Context) (json.RawMessage, error) {
	raw, err := os.ReadFile(a.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s não é um JSON válido", a.Path)
	}
	return json.RawMessage(raw), nil
}

// Restore implementa ccaccount.Settings.
func (a *SettingsFile) Restore(_ context.Context, snapshot json.RawMessage) error {
	doc := map[string]json.RawMessage{}
	if len(snapshot) > 0 {
		if err := json.Unmarshal(snapshot, &doc); err != nil {
			return fmt.Errorf("as preferências guardadas no perfil estão ilegíveis: %w", err)
		}
		if doc == nil {
			doc = map[string]json.RawMessage{}
		}
	}
	if err := a.neutralize(doc); err != nil {
		return err
	}
	return a.write(doc)
}

// neutralize anula as autenticações que o perfil não define mas que o shell
// exportou. Sem isso um `export ANTHROPIC_API_KEY` no .zshrc venceria em
// silêncio o login que o usuário acabou de ativar — e a tool anunciaria uma
// troca que a CLI não faria. Valor vazio no settings é o mecanismo oficial
// para isso.
func (a *SettingsFile) neutralize(doc map[string]json.RawMessage) error {
	env := map[string]string{}
	if raw, ok := doc["env"]; ok {
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("bloco env do perfil inválido: %w", err)
		}
		if env == nil {
			env = map[string]string{}
		}
	}

	changed := false
	for _, key := range managedAuthEnv {
		if _, defined := env[key]; defined {
			continue
		}
		if _, exported := a.lookup(key); !exported {
			continue
		}
		env[key] = ""
		changed = true
	}
	if !changed {
		return nil
	}

	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	doc["env"] = raw
	return nil
}

// ApplyAuth implementa ccaccount.Settings: troca a autenticação e preserva o
// resto do arquivo. É o caminho dos perfis anteriores ao snapshot.
func (a *SettingsFile) ApplyAuth(_ context.Context, method ccaccount.AuthMethod, value string) error {
	if method != ccaccount.AuthClaudeLogin && value == "" {
		return errors.New("valor da autenticação direta está vazio")
	}
	doc, env, err := a.read()
	if err != nil {
		return err
	}
	for _, key := range managedAuthEnv {
		env[key] = ""
	}
	delete(doc, "apiKeyHelper")

	switch method {
	case ccaccount.AuthClaudeLogin:
	case ccaccount.AuthOAuthToken:
		env[oauthTokenEnv] = value
	case ccaccount.AuthAPIKey:
		env[apiKeyEnv] = value
	case ccaccount.AuthBearerToken:
		env[authTokenEnv] = value
	case ccaccount.AuthAPIHelper:
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		doc["apiKeyHelper"] = raw
	default:
		return fmt.Errorf("método de autenticação desconhecido: %q", method)
	}

	rawEnv, err := json.Marshal(env)
	if err != nil {
		return err
	}
	doc["env"] = rawEnv
	return a.write(doc)
}

// Auth implementa ccaccount.Settings, na mesma ordem de precedência da CLI.
func (a *SettingsFile) Auth(context.Context) (ccaccount.AuthMethod, string, string, error) {
	doc, env, err := a.read()
	if err != nil {
		return ccaccount.AuthClaudeLogin, "", "", err
	}

	type candidate struct {
		method ccaccount.AuthMethod
		env    string
	}
	for _, item := range []candidate{
		{ccaccount.AuthBearerToken, authTokenEnv},
		{ccaccount.AuthAPIKey, apiKeyEnv},
	} {
		if value, origin := a.envValue(env, item.env); value != "" {
			return item.method, value, origin, nil
		}
	}

	if raw, ok := doc["apiKeyHelper"]; ok {
		var helper string
		if err := json.Unmarshal(raw, &helper); err != nil {
			return ccaccount.AuthClaudeLogin, "", "", fmt.Errorf("apiKeyHelper inválido em %s: %w", a.Path, err)
		}
		if helper != "" {
			return ccaccount.AuthAPIHelper, helper, a.Path, nil
		}
	}

	if value, origin := a.envValue(env, oauthTokenEnv); value != "" {
		return ccaccount.AuthOAuthToken, value, origin, nil
	}
	return ccaccount.AuthClaudeLogin, "", "", ccaccount.ErrNoDirectAuth
}

func (a *SettingsFile) envValue(settings map[string]string, key string) (string, string) {
	if value, ok := settings[key]; ok {
		return value, a.Path
	}
	if value, ok := a.lookup(key); ok {
		return value, "variável " + key
	}
	return "", ""
}

func (a *SettingsFile) lookup(key string) (string, bool) {
	if a.getenv == nil {
		return os.LookupEnv(key)
	}
	return a.getenv(key)
}

func (a *SettingsFile) read() (map[string]json.RawMessage, map[string]string, error) {
	raw, err := os.ReadFile(a.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, map[string]string{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	env := map[string]string{}
	if rawEnv, ok := doc["env"]; ok {
		if err := json.Unmarshal(rawEnv, &env); err != nil {
			return nil, nil, fmt.Errorf("env inválido em %s: %w", a.Path, err)
		}
		if env == nil {
			env = map[string]string{}
		}
	}
	return doc, env, nil
}

func (a *SettingsFile) write(doc map[string]json.RawMessage) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := xdg.MkdirAll(filepath.Dir(a.Path), 0o700); err != nil {
		return err
	}
	return writeAtomic(a.Path, buf.Bytes(), 0o600)
}
