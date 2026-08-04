package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
)

func TestSettingsAuthAplicaMetodosSemApagarOutrasPreferencias(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "permissions": {"allow": ["Read"]},
  "env": {"FOO": "bar", "ANTHROPIC_AUTH_TOKEN": "antigo"},
  "apiKeyHelper": "helper-antigo"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := NewSettingsAuth(path)
	auth.getenv = func(key string) (string, bool) {
		return "export-do-shell-" + key, true
	}

	tests := []struct {
		method ccaccount.AuthMethod
		value  string
	}{
		{ccaccount.AuthAPIKey, "sk-ant-api03-segredo"},
		{ccaccount.AuthBearerToken, "bearer-segredo"},
		{ccaccount.AuthOAuthToken, "oauth-segredo"},
		{ccaccount.AuthAPIHelper, "C:\\tools\\gera-chave.cmd"},
	}
	for _, test := range tests {
		if err := auth.Apply(context.Background(), test.method, test.value); err != nil {
			t.Fatalf("Apply(%s): %v", test.method, err)
		}
		method, value, origin, err := auth.Read(context.Background())
		if err != nil || method != test.method || value != test.value || origin != path {
			t.Fatalf("Read depois de %s = (%s, %q, %q, %v)", test.method, method, value, origin, err)
		}

		var doc map[string]json.RawMessage
		raw, _ := os.ReadFile(path)
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if _, ok := doc["permissions"]; !ok {
			t.Error("a troca apagou permissions")
		}
		var env map[string]string
		if err := json.Unmarshal(doc["env"], &env); err != nil {
			t.Fatal(err)
		}
		if env["FOO"] != "bar" {
			t.Error("a troca apagou variável não relacionada")
		}
	}

	if err := auth.Apply(context.Background(), ccaccount.AuthClaudeLogin, ""); err != nil {
		t.Fatalf("Apply(login): %v", err)
	}
	if _, _, _, err := auth.Read(context.Background()); !errors.Is(err, ccaccount.ErrNoDirectAuth) {
		t.Fatalf("exports do shell deviam estar anulados, veio %v", err)
	}
}

func TestSettingsAuthRespeitaPrecedenciaOficial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{
  "env": {"CLAUDE_CODE_OAUTH_TOKEN": "oauth"},
  "apiKeyHelper": "gera-chave"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := NewSettingsAuth(path)
	auth.getenv = func(key string) (string, bool) {
		if key == authTokenEnv {
			return "bearer", true
		}
		return "", false
	}

	method, value, _, err := auth.Read(context.Background())
	if err != nil || method != ccaccount.AuthBearerToken || value != "bearer" {
		t.Fatalf("precedência = (%s, %q, %v)", method, value, err)
	}
}
