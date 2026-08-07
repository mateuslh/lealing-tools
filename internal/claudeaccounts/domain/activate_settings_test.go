package ccaccount_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	claudecli "github.com/mateuslh/lealing-tools/internal/claudeaccounts/adapter"
	ccaccount "github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
)

// Com o adapter real de settings, restaurar um perfil reescreve o arquivo pelo
// encoder da plataforma, que reordena as chaves do mapa. A conferência de
// installedAs precisa aprovar essa troca correta: exigir bytes idênticos
// reprovava a volta para uma conta de login só porque o arquivo relido tinha
// as mesmas chaves em outra ordem — o sintoma "a sessão conferida depois da
// troca difere do perfil escolhido".
func TestAtivarAprovaTrocaComSettingsReordenado(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Ordem não-alfabética, como a CLI do Claude Code escreve.
	loginJSON := []byte(`{"model":"opus","includeCoAuthoredBy":false,"env":{"ZED":"1","ALFA":"2"}}`)
	if err := os.WriteFile(settingsPath, loginJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	settings := claudecli.NewSettingsFile(settingsPath)
	vault := &fakeVault{cred: credential("pro", now().Add(time.Hour), now().AddDate(0, 0, 30))}
	config := &fakeConfig{account: account("eu@exemplo.com", "Pessoal", "uuid-pessoal"), userID: "u"}
	store := newFakeStore()
	m := ccaccount.NewManager(vault, config, store, now)
	m.WithSettings(settings)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}

	// Vira uma conta com token Bearer ativa, gravando o token no settings.
	bearerJSON := []byte(`{"model":"sonnet","env":{"ANTHROPIC_AUTH_TOKEN":"tok-bearer"}}`)
	if err := os.WriteFile(settingsPath, bearerJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(ctx, "bearer"); err != nil {
		t.Fatalf("Save bearer: %v", err)
	}

	// Voltar para o login não pode ser reprovado só porque o arquivo relido tem
	// as mesmas chaves em outra ordem depois do Restore.
	if err := m.Activate(ctx, "pessoal"); err != nil {
		t.Fatalf("Activate pessoal: %v", err)
	}
}
