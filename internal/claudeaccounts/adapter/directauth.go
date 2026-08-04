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

// SettingsAuth alterna as autenticações diretas pelo mecanismo oficial de
// env e apiKeyHelper do settings.json. Valores vazios são intencionais: eles
// anulam exports herdados do shell para o método escolhido realmente vencer.
type SettingsAuth struct {
	Path   string
	getenv func(string) (string, bool)
}

var _ ccaccount.DirectAuth = (*SettingsAuth)(nil)

func NewSettingsAuth(path string) *SettingsAuth {
	return &SettingsAuth{Path: path, getenv: os.LookupEnv}
}

// Read devolve o método efetivo na mesma ordem de precedência da CLI.
func (a *SettingsAuth) Read(context.Context) (ccaccount.AuthMethod, string, string, error) {
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

// Apply seleciona um único método e desabilita explicitamente os demais.
func (a *SettingsAuth) Apply(_ context.Context, method ccaccount.AuthMethod, value string) error {
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

func (a *SettingsAuth) envValue(settings map[string]string, key string) (string, string) {
	if value, ok := settings[key]; ok {
		return value, a.Path
	}
	lookup := a.getenv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if value, ok := lookup(key); ok {
		return value, "variável " + key
	}
	return "", ""
}

func (a *SettingsAuth) read() (map[string]json.RawMessage, map[string]string, error) {
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

func (a *SettingsAuth) write(doc map[string]json.RawMessage) error {
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
