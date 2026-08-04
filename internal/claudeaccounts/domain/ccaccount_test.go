package ccaccount_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mateuslh/lealing-tools/internal/claudeaccounts/domain"
)

func now() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) }

// credential monta a credencial como a CLI a grava, com as duas validades em
// milissegundos.
func credential(plan string, expires, renews time.Time) json.RawMessage {
	payload := map[string]any{"claudeAiOauth": map[string]any{
		"accessToken":           "tok-" + plan,
		"refreshToken":          "ref-" + plan,
		"subscriptionType":      plan,
		"expiresAt":             expires.UnixMilli(),
		"refreshTokenExpiresAt": renews.UnixMilli(),
	}}
	raw, _ := json.Marshal(payload)
	return raw
}

func account(email, org, uuid string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"emailAddress": email, "organizationName": org,
		"accountUuid": uuid, "displayName": "Alguém",
	})
	return raw
}

// --- Duplos ------------------------------------------------------------

type fakeVault struct {
	cred           json.RawMessage
	err            error
	writeErr       error
	failAfterWrite bool
}

func (f *fakeVault) Read(context.Context) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.cred) == 0 {
		return nil, ccaccount.ErrNoActiveSession
	}
	return f.cred, nil
}

func (f *fakeVault) Write(_ context.Context, c json.RawMessage) error {
	err := f.writeErr
	f.writeErr = nil
	if err == nil || f.failAfterWrite {
		f.cred = c
	}
	return err
}
func (f *fakeVault) Origin() string { return "cofre de teste" }

type fakeConfig struct {
	account json.RawMessage
	userID  string
	setErr  error
}

type fakeDirectAuth struct {
	method         ccaccount.AuthMethod
	value          string
	origin         string
	err            error
	applyErr       error
	failAfterApply bool
}

func (f *fakeDirectAuth) Read(context.Context) (ccaccount.AuthMethod, string, string, error) {
	if f.err != nil {
		return ccaccount.AuthClaudeLogin, "", "", f.err
	}
	if f.method == ccaccount.AuthClaudeLogin {
		return ccaccount.AuthClaudeLogin, "", "", ccaccount.ErrNoDirectAuth
	}
	return f.method, f.value, f.origin, nil
}

func (f *fakeDirectAuth) Apply(_ context.Context, method ccaccount.AuthMethod, value string) error {
	err := f.applyErr
	f.applyErr = nil
	if err == nil || f.failAfterApply {
		f.method, f.value = method, value
		if method == ccaccount.AuthClaudeLogin {
			f.value = ""
		}
	}
	return err
}

func (f *fakeConfig) Account(context.Context) (json.RawMessage, string, error) {
	return f.account, f.userID, nil
}

func (f *fakeConfig) SetAccount(_ context.Context, a json.RawMessage, id string) error {
	if f.setErr != nil {
		err := f.setErr
		f.setErr = nil
		return err
	}
	f.account, f.userID = a, id
	return nil
}

type fakeStore struct {
	profiles map[string]ccaccount.Profile
	sessions map[string]ccaccount.Session
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		profiles: map[string]ccaccount.Profile{},
		sessions: map[string]ccaccount.Session{},
	}
}

func (f *fakeStore) List(context.Context) ([]ccaccount.Profile, error) {
	out := make([]ccaccount.Profile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeStore) Load(_ context.Context, name string) (ccaccount.Session, error) {
	s, ok := f.sessions[name]
	if !ok {
		return ccaccount.Session{}, ccaccount.ErrEmptyProfile
	}
	return s, nil
}

func (f *fakeStore) Save(_ context.Context, p ccaccount.Profile, s ccaccount.Session) error {
	f.profiles[p.Name], f.sessions[p.Name] = p, s
	return nil
}

func (f *fakeStore) Delete(_ context.Context, name string) error {
	delete(f.profiles, name)
	delete(f.sessions, name)
	return nil
}

// setup monta o gerente com uma sessão pessoal ativa.
func setup(t *testing.T) (*ccaccount.Manager, *fakeVault, *fakeConfig, *fakeStore) {
	t.Helper()
	vault := &fakeVault{cred: credential("pro", now().Add(time.Hour), now().AddDate(0, 0, 30))}
	config := &fakeConfig{account: account("eu@exemplo.com", "Pessoal", "uuid-pessoal"), userID: "user-pessoal"}
	store := newFakeStore()
	return ccaccount.NewManager(vault, config, store, now), vault, config, store
}

// --- Testes ------------------------------------------------------------

func TestEstadoDescreveContaAtivaEPerfilQueACobre(t *testing.T) {
	m, _, _, _ := setup(t)
	ctx := context.Background()

	st, err := m.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !st.HasActive {
		t.Fatal("sessão ativa não foi reconhecida")
	}
	if st.Active.Email != "eu@exemplo.com" || st.Active.Plan != "pro" {
		t.Errorf("identidade lida errada: %+v", st.Active)
	}
	if st.ActiveProfile != "" {
		t.Errorf("sessão ainda não salva foi dada como guardada em %q", st.ActiveProfile)
	}

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st, _ = m.State(ctx)
	if st.ActiveProfile != "pessoal" {
		t.Errorf("depois de salvar, o perfil ativo devia ser “pessoal”, veio %q", st.ActiveProfile)
	}
}

func TestSemSessaoAtivaOEstadoAindaAbre(t *testing.T) {
	m := ccaccount.NewManager(&fakeVault{}, &fakeConfig{}, newFakeStore(), now)

	st, err := m.State(context.Background())
	if err != nil {
		t.Fatalf("faltar sessão não pode ser erro: %v", err)
	}
	if st.HasActive {
		t.Error("nenhuma credencial, mas o estado diz que há sessão")
	}
}

func TestSalvarRecusaSobrescreverOutraContaSemConfirmacao(t *testing.T) {
	m, vault, config, _ := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "trabalho"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A CLI passa a apontar para outra conta, e o usuário tenta guardá-la
	// sob o mesmo nome.
	vault.cred = credential("max", now().Add(time.Hour), now().AddDate(0, 0, 30))
	config.account = account("eu@empresa.com", "Empresa", "uuid-empresa")

	if _, err := m.Save(ctx, "trabalho"); !errors.Is(err, ccaccount.ErrProfileExists) {
		t.Fatalf("esperava ErrProfileExists, veio %v", err)
	}
	if _, err := m.SaveOverwriting(ctx, "trabalho"); err != nil {
		t.Fatalf("SaveOverwriting: %v", err)
	}

	st, _ := m.State(ctx)
	if st.ActiveProfile != "trabalho" {
		t.Errorf("depois de confirmar, o perfil devia cobrir a conta nova: %+v", st)
	}
}

func TestAtivarProtegeSessaoNaoGuardada(t *testing.T) {
	m, vault, config, _ := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Login em outra conta, fora do lealing: a sessão ativa deixa de estar
	// coberta por qualquer perfil.
	vault.cred = credential("max", now().Add(time.Hour), now().AddDate(0, 0, 30))
	config.account = account("eu@empresa.com", "Empresa", "uuid-empresa")

	if err := m.Activate(ctx, "pessoal"); !errors.Is(err, ccaccount.ErrActiveUnsaved) {
		t.Fatalf("esperava ErrActiveUnsaved, veio %v", err)
	}
	if string(config.account) == "" || string(vault.cred) == "" {
		t.Fatal("a recusa não pode ter tocado no cofre")
	}
	if got := string(config.account); got != string(account("eu@empresa.com", "Empresa", "uuid-empresa")) {
		t.Errorf("a identidade foi alterada apesar da recusa: %s", got)
	}
}

func TestAtivarRestauraCredencialEIdentidade(t *testing.T) {
	m, vault, config, _ := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	pessoal := vault.cred

	vault.cred = credential("max", now().Add(time.Hour), now().AddDate(0, 0, 30))
	config.account = account("eu@empresa.com", "Empresa", "uuid-empresa")
	config.userID = "user-empresa"
	if _, err := m.Save(ctx, "trabalho"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := m.Activate(ctx, "pessoal"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if string(vault.cred) != string(pessoal) {
		t.Error("a credencial do perfil não voltou ao cofre")
	}
	if config.userID != "user-pessoal" {
		t.Errorf("userID não acompanhou a troca: %q", config.userID)
	}

	st, _ := m.State(ctx)
	if st.Active.Email != "eu@exemplo.com" || st.ActiveProfile != "pessoal" {
		t.Errorf("estado após a troca: %+v", st)
	}
}

func TestEstadoAtualizaTokenRotacionadoDoPerfilAtivo(t *testing.T) {
	m, vault, _, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rotated := credential("pro", now().Add(6*time.Hour), now().AddDate(0, 1, 0))
	vault.cred = rotated

	st, err := m.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.ActiveProfile != "pessoal" {
		t.Fatalf("perfil ativo = %q", st.ActiveProfile)
	}
	if got := store.sessions["pessoal"].Credential; string(got) != string(rotated) {
		t.Error("o perfil continuou com o refresh token anterior à rotação")
	}
	if !st.Profiles[0].Identity.ExpiresAt.Equal(now().Add(6 * time.Hour)) {
		t.Errorf("metadados do perfil não acompanharam a rotação: %+v", st.Profiles[0].Identity)
	}
}

func TestAtivarPreservaTokenRotacionadoMesmoSemRecarregarTela(t *testing.T) {
	m, vault, config, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}

	workSession := ccaccount.Session{
		Credential: credential("max", now().Add(2*time.Hour), now().AddDate(0, 1, 0)),
		Account:    account("eu@empresa.com", "Empresa", "uuid-empresa"),
		UserID:     "user-empresa",
	}
	workProfile := ccaccount.Profile{
		Name: "trabalho", Identity: ccaccount.Describe(workSession), SavedAt: now(),
	}
	if err := store.Save(ctx, workProfile, workSession); err != nil {
		t.Fatal(err)
	}

	rotated := credential("pro", now().Add(8*time.Hour), now().AddDate(0, 2, 0))
	vault.cred = rotated
	if err := m.Activate(ctx, "trabalho"); err != nil {
		t.Fatalf("Activate trabalho: %v", err)
	}
	if config.userID != "user-empresa" {
		t.Fatalf("trabalho não foi ativado: %q", config.userID)
	}
	if err := m.Activate(ctx, "pessoal"); err != nil {
		t.Fatalf("Activate pessoal: %v", err)
	}
	if string(vault.cred) != string(rotated) {
		t.Error("voltar ao perfil restaurou o refresh token revogado")
	}
}

func TestTrocaNoWindowsMarcaCredencialMesmoComIdentidadeAntiga(t *testing.T) {
	m, _, config, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}
	// A extensão do Claude pode trocar a credencial sem atualizar o
	// oauthAccount. Os dois perfis ficam com a mesma identidade aparente,
	// mas suas credenciais continuam sendo inequivocamente diferentes.
	workSession := ccaccount.Session{
		Credential: credential("max", now().Add(2*time.Hour), now().AddDate(0, 1, 0)),
		Account:    append(json.RawMessage(nil), config.account...),
		UserID:     config.userID,
	}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "trabalho", Identity: ccaccount.Describe(workSession), SavedAt: now(),
	}, workSession); err != nil {
		t.Fatal(err)
	}

	if err := m.Activate(ctx, "trabalho"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	st, err := m.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.ActiveProfile != "trabalho" {
		t.Errorf("perfil verde = %q, queria trabalho", st.ActiveProfile)
	}
}

func TestEstadoNaoContaminaPerfilComIdentidadeAntigaDoWindows(t *testing.T) {
	m, vault, _, store := setup(t)
	ctx := context.Background()

	workSession := ccaccount.Session{
		Credential: credential("max", now().Add(2*time.Hour), now().AddDate(0, 1, 0)),
		Account:    account("eu@empresa.com", "Empresa", "uuid-empresa"),
		UserID:     "user-empresa",
	}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "trabalho", Identity: ccaccount.Describe(workSession), SavedAt: now(),
	}, workSession); err != nil {
		t.Fatal(err)
	}

	// A credencial já é a de trabalho, mas o fakeConfig continua com a
	// identidade pessoal montada por setup — situação observada no Windows.
	vault.cred = workSession.Credential
	st, err := m.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if st.ActiveProfile != "trabalho" {
		t.Fatalf("perfil ativo = %q", st.ActiveProfile)
	}
	stored := store.sessions["trabalho"]
	if string(stored.Account) != string(workSession.Account) || stored.UserID != "user-empresa" {
		t.Errorf("identidade boa foi substituída pela antiga: %+v", stored)
	}
}

func TestAtivarReverteCredencialQuandoIdentidadeFalha(t *testing.T) {
	m, vault, config, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}
	originalCredential := append(json.RawMessage(nil), vault.cred...)
	originalAccount := append(json.RawMessage(nil), config.account...)

	workSession := ccaccount.Session{
		Credential: credential("max", now().Add(2*time.Hour), now().AddDate(0, 1, 0)),
		Account:    account("eu@empresa.com", "Empresa", "uuid-empresa"),
		UserID:     "user-empresa",
	}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "trabalho", Identity: ccaccount.Describe(workSession), SavedAt: now(),
	}, workSession); err != nil {
		t.Fatal(err)
	}

	config.setErr = errors.New("disco indisponível")
	if err := m.Activate(ctx, "trabalho"); err == nil {
		t.Fatal("Activate devia falhar")
	}
	if string(vault.cred) != string(originalCredential) {
		t.Error("a falha deixou a credencial da conta alvo parcialmente aplicada")
	}
	if string(config.account) != string(originalAccount) || config.userID != "user-pessoal" {
		t.Error("a falha não restaurou a identidade anterior")
	}
}

func TestAtivarReverteEscritaIncertaDoCofre(t *testing.T) {
	m, vault, _, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}
	original := append(json.RawMessage(nil), vault.cred...)
	workSession := ccaccount.Session{
		Credential: credential("max", now().Add(2*time.Hour), now().AddDate(0, 1, 0)),
		Account:    account("eu@empresa.com", "Empresa", "uuid-empresa"),
		UserID:     "user-empresa",
	}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "trabalho", Identity: ccaccount.Describe(workSession), SavedAt: now(),
	}, workSession); err != nil {
		t.Fatal(err)
	}

	// Simula um cofre que efetivou a escrita, mas falhou ao reler para
	// confirmá-la — o caso mais perigoso do security no macOS.
	vault.writeErr = errors.New("não foi possível conferir")
	vault.failAfterWrite = true
	if err := m.Activate(ctx, "trabalho"); err == nil {
		t.Fatal("Activate devia falhar")
	}
	if string(vault.cred) != string(original) {
		t.Error("a escrita incerta não foi revertida para a credencial anterior")
	}
}

func TestAtivarPerfilInexistente(t *testing.T) {
	m, _, _, _ := setup(t)
	if err := m.Activate(context.Background(), "fantasma"); !errors.Is(err, ccaccount.ErrProfileNotFound) {
		t.Fatalf("esperava ErrProfileNotFound, veio %v", err)
	}
}

func TestAlternaTodosOsMetodosDeAutenticacaoDireta(t *testing.T) {
	m, vault, _, store := setup(t)
	direct := &fakeDirectAuth{origin: "settings.json"}
	m.WithDirectAuth(direct)
	ctx := context.Background()

	if _, err := m.Save(ctx, "login"); err != nil {
		t.Fatalf("Save login: %v", err)
	}
	loginCredential := append(json.RawMessage(nil), vault.cred...)

	tests := []struct {
		name   string
		method ccaccount.AuthMethod
		value  string
	}{
		{"oauth-token", ccaccount.AuthOAuthToken, "oauth-segredo"},
		{"api-key", ccaccount.AuthAPIKey, "api-segredo"},
		{"bearer", ccaccount.AuthBearerToken, "bearer-segredo"},
		{"helper", ccaccount.AuthAPIHelper, "gera-chave --perfil trabalho"},
	}
	for _, test := range tests {
		direct.method, direct.value = test.method, test.value
		profile, err := m.Save(ctx, test.name)
		if err != nil {
			t.Fatalf("Save %s: %v", test.name, err)
		}
		if profile.Method != test.method {
			t.Errorf("método do perfil %s = %s", test.name, profile.Method)
		}
	}

	for _, test := range tests {
		if err := m.Activate(ctx, test.name); err != nil {
			t.Fatalf("Activate %s: %v", test.name, err)
		}
		if direct.method != test.method || direct.value != test.value {
			t.Errorf("%s aplicou (%s, %q)", test.name, direct.method, direct.value)
		}
		st, err := m.State(ctx)
		if err != nil || st.ActiveProfile != test.name || st.Method != test.method {
			t.Errorf("estado de %s = %+v (%v)", test.name, st, err)
		}
	}

	if err := m.Activate(ctx, "login"); err != nil {
		t.Fatalf("Activate login: %v", err)
	}
	if direct.method != ccaccount.AuthClaudeLogin || direct.value != "" {
		t.Error("voltar ao login não desabilitou as autenticações diretas")
	}
	if string(vault.cred) != string(loginCredential) {
		t.Error("voltar ao login não restaurou a credencial OAuth persistida")
	}
	if got := store.sessions["api-key"]; got.AuthValue != "api-segredo" {
		t.Error("API key não ficou protegida junto da sessão")
	}
}

func TestAtivarReverteEscritaIncertaDaAutenticacaoDireta(t *testing.T) {
	m, _, _, store := setup(t)
	direct := &fakeDirectAuth{
		method: ccaccount.AuthAPIKey, value: "api-anterior", origin: "settings.json",
	}
	m.WithDirectAuth(direct)
	ctx := context.Background()
	if _, err := m.Save(ctx, "api"); err != nil {
		t.Fatal(err)
	}
	bearer := ccaccount.Session{Method: ccaccount.AuthBearerToken, AuthValue: "bearer-novo"}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "bearer", Method: bearer.Method, Identity: ccaccount.Describe(bearer), SavedAt: now(),
	}, bearer); err != nil {
		t.Fatal(err)
	}

	direct.applyErr = errors.New("não foi possível conferir settings.json")
	direct.failAfterApply = true
	if err := m.Activate(ctx, "bearer"); err == nil {
		t.Fatal("Activate devia falhar")
	}
	if direct.method != ccaccount.AuthAPIKey || direct.value != "api-anterior" {
		t.Errorf("autenticação anterior não foi restaurada: (%s, %q)", direct.method, direct.value)
	}
}

func TestEsquecerNaoDeslogaAContaEmUso(t *testing.T) {
	m, vault, _, store := setup(t)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	before := string(vault.cred)

	if err := m.Forget(ctx, "pessoal"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if len(store.profiles) != 0 {
		t.Error("o perfil continuou no store")
	}
	if string(vault.cred) != before {
		t.Error("esquecer o perfil mexeu na credencial ativa")
	}
}

func TestValidadeDaSessaoSeparaRenovavelDeMorta(t *testing.T) {
	renovavel := ccaccount.Describe(ccaccount.Session{
		Credential: credential("pro", now().Add(-time.Hour), now().AddDate(0, 0, 10)),
	})
	if !renovavel.Stale(now()) || renovavel.Dead(now()) {
		t.Error("token vencido com refresh válido devia ser só “renovável”")
	}

	morta := ccaccount.Describe(ccaccount.Session{
		Credential: credential("pro", now().Add(-48*time.Hour), now().Add(-time.Hour)),
	})
	if !morta.Dead(now()) {
		t.Error("refresh vencido devia exigir login novo")
	}
}

func TestNomeDePerfilRecusaOQueOCofreNaoAceita(t *testing.T) {
	casos := map[string]bool{
		"pessoal":       true,
		"conta de casa": true,
		"trabalho-2":    true,
		" espaços  ":    true, // é aparado, não recusado
		"":              false,
		"   ":           false,
		"caminho/ruim":  false,
		"dois:pontos":   false,
	}
	for entrada, ok := range casos {
		_, err := ccaccount.NormalizeName(entrada)
		if ok && err != nil {
			t.Errorf("%q devia ser aceito: %v", entrada, err)
		}
		if !ok && err == nil {
			t.Errorf("%q devia ser recusado", entrada)
		}
	}

	if got, _ := ccaccount.NormalizeName("  pessoal  "); got != "pessoal" {
		t.Errorf("espaços das pontas deviam sumir, veio %q", got)
	}
}

func TestSessaoIlegivelNaoDerrubaALeitura(t *testing.T) {
	id := ccaccount.Describe(ccaccount.Session{
		Credential: json.RawMessage(`{"claudeAiOauth":`), // truncado
		Account:    json.RawMessage(`não é json`),
	})
	if !id.Empty() {
		t.Errorf("blob quebrado devia render identidade vazia, veio %+v", id)
	}
}
