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
	caches  map[string]json.RawMessage
	setErr  error
}

func (f *fakeConfig) Account(context.Context) (json.RawMessage, string, map[string]json.RawMessage, error) {
	return f.account, f.userID, f.caches, nil
}

func (f *fakeConfig) SetAccount(_ context.Context, a json.RawMessage, id string, caches map[string]json.RawMessage) error {
	if f.setErr != nil {
		err := f.setErr
		f.setErr = nil
		return err
	}
	f.account, f.userID, f.caches = a, id, caches
	return nil
}

// fakeSettings imita o settings.json: guarda o documento inteiro e deriva
// dele a autenticação efetiva, para que Snapshot e Restore fechem a volta.
// O campo "extra" faz o papel do ANTHROPIC_BASE_URL — configuração que é da
// conta sem ser autenticação, e que precisa viajar junto.
type fakeSettings struct {
	doc              json.RawMessage
	origin           string
	snapshotErr      error
	restoreErr       error
	failAfterRestore bool
}

func (f *fakeSettings) set(method ccaccount.AuthMethod, value, extra string) {
	raw, _ := json.Marshal(map[string]string{
		"method": string(method), "value": value, "extra": extra,
	})
	f.doc = raw
}

func (f *fakeSettings) parse() (method ccaccount.AuthMethod, value, extra string) {
	var d struct{ Method, Value, Extra string }
	_ = json.Unmarshal(f.doc, &d)
	return ccaccount.AuthMethod(d.Method), d.Value, d.Extra
}

func (f *fakeSettings) Snapshot(context.Context) (json.RawMessage, error) {
	return f.doc, f.snapshotErr
}

func (f *fakeSettings) Restore(_ context.Context, doc json.RawMessage) error {
	err := f.restoreErr
	f.restoreErr = nil
	if err == nil || f.failAfterRestore {
		f.doc = doc
	}
	return err
}

func (f *fakeSettings) ApplyAuth(_ context.Context, method ccaccount.AuthMethod, value string) error {
	_, _, extra := f.parse()
	f.set(method, value, extra)
	return nil
}

func (f *fakeSettings) Auth(context.Context) (ccaccount.AuthMethod, string, string, error) {
	method, value, _ := f.parse()
	if method == ccaccount.AuthClaudeLogin {
		return ccaccount.AuthClaudeLogin, "", "", ccaccount.ErrNoDirectAuth
	}
	return method, value, f.origin, nil
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
	settings := &fakeSettings{origin: "settings.json"}
	settings.set(ccaccount.AuthClaudeLogin, "", "config-do-login")
	m.WithSettings(settings)
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
		settings.set(test.method, test.value, "config-de-"+test.name)
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
		method, value, extra := settings.parse()
		if method != test.method || value != test.value {
			t.Errorf("%s aplicou (%s, %q)", test.name, method, value)
		}
		if want := "config-de-" + test.name; extra != want {
			t.Errorf("%s: configuração da conta ficou %q, esperava %q", test.name, extra, want)
		}
		st, err := m.State(ctx)
		if err != nil || st.ActiveProfile != test.name || st.Method != test.method {
			t.Errorf("estado de %s = %+v (%v)", test.name, st, err)
		}
	}

	if err := m.Activate(ctx, "login"); err != nil {
		t.Fatalf("Activate login: %v", err)
	}
	method, value, extra := settings.parse()
	if method != ccaccount.AuthClaudeLogin || value != "" {
		t.Error("voltar ao login não desabilitou as autenticações diretas")
	}
	if extra != "config-do-login" {
		t.Errorf("voltar ao login deixou a configuração da conta anterior: %q", extra)
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
	settings := &fakeSettings{origin: "settings.json"}
	settings.set(ccaccount.AuthAPIKey, "api-anterior", "config-da-api")
	m.WithSettings(settings)
	ctx := context.Background()
	if _, err := m.Save(ctx, "api"); err != nil {
		t.Fatal(err)
	}

	alvo := &fakeSettings{}
	alvo.set(ccaccount.AuthBearerToken, "bearer-novo", "config-do-bearer")
	bearer := ccaccount.Session{
		Method: ccaccount.AuthBearerToken, AuthValue: "bearer-novo", Settings: alvo.doc,
	}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "bearer", Method: bearer.Method, Identity: ccaccount.Describe(bearer), SavedAt: now(),
	}, bearer); err != nil {
		t.Fatal(err)
	}

	settings.restoreErr = errors.New("não foi possível conferir settings.json")
	settings.failAfterRestore = true
	if err := m.Activate(ctx, "bearer"); err == nil {
		t.Fatal("Activate devia falhar")
	}
	method, value, extra := settings.parse()
	if method != ccaccount.AuthAPIKey || value != "api-anterior" {
		t.Errorf("autenticação anterior não foi restaurada: (%s, %q)", method, value)
	}
	if extra != "config-da-api" {
		t.Errorf("configuração anterior não foi restaurada: %q", extra)
	}
}

// A configuração de uma conta não é só o token: um gateway traz endpoint e
// nomes de modelo próprios, e voltar para a conta de login com o endpoint da
// outra no lugar quebra as duas.
func TestTrocaDeContaLevaAConfiguracaoInteiraJunto(t *testing.T) {
	m, vault, config, _ := setup(t)
	settings := &fakeSettings{origin: "settings.json"}
	settings.set(ccaccount.AuthClaudeLogin, "", "api.anthropic.com")
	m.WithSettings(settings)
	ctx := context.Background()

	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}

	// A conta do trabalho passa por um gateway, com chave e endereço próprios.
	settings.set(ccaccount.AuthAPIKey, "chave-do-gateway", "gateway.empresa.com")
	vault.cred = credential("team", now().Add(time.Hour), now().AddDate(0, 0, 30))
	config.account = account("eu@empresa.com", "Empresa", "uuid-empresa")
	if _, err := m.Save(ctx, "trabalho"); err != nil {
		t.Fatalf("Save trabalho: %v", err)
	}

	if err := m.Activate(ctx, "pessoal"); err != nil {
		t.Fatalf("Activate pessoal: %v", err)
	}
	if _, _, extra := settings.parse(); extra != "api.anthropic.com" {
		t.Errorf("voltar à conta pessoal manteve o endereço do gateway: %q", extra)
	}

	if err := m.Activate(ctx, "trabalho"); err != nil {
		t.Fatalf("Activate trabalho: %v", err)
	}
	method, value, extra := settings.parse()
	if method != ccaccount.AuthAPIKey || value != "chave-do-gateway" || extra != "gateway.empresa.com" {
		t.Errorf("a conta do gateway não voltou inteira: (%s, %q, %q)", method, value, extra)
	}
}

// Cotas e modelos liberados são da conta. Guardá-los faz a conta voltar como
// estava; apagá-los fazia cada troca parecer um primeiro acesso.
func TestCachesDaContaVoltamComOPerfil(t *testing.T) {
	m, vault, config, _ := setup(t)
	ctx := context.Background()

	config.caches = map[string]json.RawMessage{"cachedUsageUtilization": json.RawMessage(`{"pct":10}`)}
	if _, err := m.Save(ctx, "pessoal"); err != nil {
		t.Fatalf("Save pessoal: %v", err)
	}

	vault.cred = credential("team", now().Add(time.Hour), now().AddDate(0, 0, 30))
	config.account = account("eu@empresa.com", "Empresa", "uuid-empresa")
	config.caches = map[string]json.RawMessage{"cachedUsageUtilization": json.RawMessage(`{"pct":90}`)}
	if _, err := m.Save(ctx, "trabalho"); err != nil {
		t.Fatalf("Save trabalho: %v", err)
	}

	if err := m.Activate(ctx, "pessoal"); err != nil {
		t.Fatalf("Activate pessoal: %v", err)
	}
	if got := string(config.caches["cachedUsageUtilization"]); got != `{"pct":10}` {
		t.Errorf("o cache de uso da conta pessoal não voltou: %s", got)
	}
}

// Perfis salvos antes de a tool guardar o settings não prometem nada sobre o
// arquivo. Restaurar um snapshot que eles não têm apagaria model, tema e
// permissões de quem só queria trocar de conta.
func TestPerfilSemSnapshotSoTrocaAAutenticacao(t *testing.T) {
	m, _, _, store := setup(t)
	settings := &fakeSettings{origin: "settings.json"}
	settings.set(ccaccount.AuthAPIKey, "chave-atual", "preferencias-do-usuario")
	m.WithSettings(settings)
	ctx := context.Background()
	if _, err := m.Save(ctx, "atual"); err != nil {
		t.Fatal(err)
	}

	legado := ccaccount.Session{Method: ccaccount.AuthBearerToken, AuthValue: "bearer-legado"}
	if err := store.Save(ctx, ccaccount.Profile{
		Name: "legado", Method: legado.Method, SavedAt: now(),
	}, legado); err != nil {
		t.Fatal(err)
	}

	if err := m.Activate(ctx, "legado"); err != nil {
		t.Fatalf("Activate legado: %v", err)
	}
	method, value, extra := settings.parse()
	if method != ccaccount.AuthBearerToken || value != "bearer-legado" {
		t.Errorf("o perfil legado não aplicou a autenticação: (%s, %q)", method, value)
	}
	if extra != "preferencias-do-usuario" {
		t.Errorf("o perfil legado apagou as preferências do usuário: %q", extra)
	}
}

// O perfil legado ganha o snapshot sozinho, na primeira vez que a conta dele
// estiver ativa — sem exigir que o usuário salve tudo de novo.
func TestPerfilSemSnapshotGanhaUmQuandoSuaContaEstaAtiva(t *testing.T) {
	m, vault, _, store := setup(t)
	settings := &fakeSettings{origin: "settings.json"}
	settings.set(ccaccount.AuthClaudeLogin, "", "config-corrente")
	m.WithSettings(settings)
	ctx := context.Background()

	// Um perfil como a versão anterior o gravava: credencial, sem settings.
	legado := ccaccount.Session{Credential: vault.cred}
	if err := store.Save(ctx, ccaccount.Profile{Name: "pessoal", SavedAt: now()}, legado); err != nil {
		t.Fatal(err)
	}

	if _, err := m.State(ctx); err != nil {
		t.Fatalf("State: %v", err)
	}
	if got := store.sessions["pessoal"]; len(got.Settings) == 0 {
		t.Error("o perfil da conta ativa não ganhou o snapshot do settings")
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
