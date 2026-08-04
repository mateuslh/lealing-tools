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

func TestSettingsApplyAuthAplicaMetodosSemApagarOutrasPreferencias(t *testing.T) {
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
	auth := NewSettingsFile(path)
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
		if err := auth.ApplyAuth(context.Background(), test.method, test.value); err != nil {
			t.Fatalf("Apply(%s): %v", test.method, err)
		}
		method, value, origin, err := auth.Auth(context.Background())
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

	if err := auth.ApplyAuth(context.Background(), ccaccount.AuthClaudeLogin, ""); err != nil {
		t.Fatalf("Apply(login): %v", err)
	}
	if _, _, _, err := auth.Auth(context.Background()); !errors.Is(err, ccaccount.ErrNoDirectAuth) {
		t.Fatalf("exports do shell deviam estar anulados, veio %v", err)
	}
}

func TestSettingsRespeitaPrecedenciaOficial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{
  "env": {"CLAUDE_CODE_OAUTH_TOKEN": "oauth"},
  "apiKeyHelper": "gera-chave"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := NewSettingsFile(path)
	auth.getenv = func(key string) (string, bool) {
		if key == authTokenEnv {
			return "bearer", true
		}
		return "", false
	}

	method, value, _, err := auth.Auth(context.Background())
	if err != nil || method != ccaccount.AuthBearerToken || value != "bearer" {
		t.Fatalf("precedência = (%s, %q, %v)", method, value, err)
	}
}

// O caso que motivou guardar o arquivo inteiro: um gateway configurado com
// endereço e nomes de modelo próprios tem de sair do lugar quando o usuário
// volta para uma conta de login comum.
func TestSnapshotERestoreTrocamOArquivoInteiro(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	gateway := []byte(`{
  "model": "opus",
  "env": {
    "ANTHROPIC_API_KEY": "chave-do-gateway",
    "ANTHROPIC_BASE_URL": "https://gateway.empresa.com",
    "ANTHROPIC_MODEL": "empresa.claude-sonnet"
  }
}`)
	if err := os.WriteFile(path, gateway, 0o600); err != nil {
		t.Fatal(err)
	}

	settings := NewSettingsFile(path)
	settings.getenv = func(string) (string, bool) { return "", false }

	doTrabalho, err := settings.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	pessoal := json.RawMessage(`{"model":"sonnet","theme":"dark"}`)
	if err := settings.Restore(ctx, pessoal); err != nil {
		t.Fatalf("Restore pessoal: %v", err)
	}

	var doc map[string]json.RawMessage
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("arquivo ficou ilegível: %v", err)
	}
	if _, ok := doc["env"]; ok {
		t.Errorf("o env do gateway sobreviveu à volta para a conta pessoal: %s", raw)
	}
	if got := string(doc["theme"]); got != `"dark"` {
		t.Errorf("as preferências da conta pessoal não entraram: %s", raw)
	}
	if _, _, _, err := settings.Auth(ctx); !errors.Is(err, ccaccount.ErrNoDirectAuth) {
		t.Errorf("a conta pessoal ficou com autenticação direta: %v", err)
	}

	if err := settings.Restore(ctx, doTrabalho); err != nil {
		t.Fatalf("Restore trabalho: %v", err)
	}
	raw, _ = os.ReadFile(path)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	if err := json.Unmarshal(doc["env"], &env); err != nil {
		t.Fatal(err)
	}
	if env["ANTHROPIC_BASE_URL"] != "https://gateway.empresa.com" ||
		env["ANTHROPIC_MODEL"] != "empresa.claude-sonnet" {
		t.Errorf("a conta do gateway não voltou inteira: %v", env)
	}
}

// Restaurar um perfil que não usa autenticação direta precisa anular o que o
// shell exportou, senão um export no .zshrc vence o login recém-ativado e a
// tool anuncia uma troca que a CLI não faz.
func TestRestoreAnulaExportsDoShellQueOPerfilNaoDefine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := NewSettingsFile(path)
	settings.getenv = func(key string) (string, bool) {
		return "do-shell", key == apiKeyEnv
	}

	if err := settings.Restore(context.Background(), json.RawMessage(`{"model":"opus"}`)); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var doc map[string]json.RawMessage
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var env map[string]string
	if err := json.Unmarshal(doc["env"], &env); err != nil {
		t.Fatalf("env não foi escrito: %s", raw)
	}
	if value, ok := env[apiKeyEnv]; !ok || value != "" {
		t.Errorf("o export do shell não foi anulado: %v", env)
	}
	if _, ok := env[authTokenEnv]; ok {
		t.Errorf("anulou uma variável que ninguém exportou: %v", env)
	}
	if _, _, _, err := settings.Auth(context.Background()); !errors.Is(err, ccaccount.ErrNoDirectAuth) {
		t.Errorf("a autenticação do shell continuou vencendo: %v", err)
	}
}

func TestSnapshotDeArquivoAusenteNaoEErro(t *testing.T) {
	settings := NewSettingsFile(filepath.Join(t.TempDir(), "settings.json"))
	doc, err := settings.Snapshot(context.Background())
	if err != nil || doc != nil {
		t.Fatalf("Snapshot sem arquivo = (%s, %v)", doc, err)
	}
}
