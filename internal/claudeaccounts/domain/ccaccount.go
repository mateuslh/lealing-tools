// Package ccaccount é o domínio da tool "Contas do Claude Code": guardar as
// sessões de mais de uma conta e devolver a escolhida ao lugar de onde a CLI
// a lê.
//
// Trocar de conta é mover duas coisas em conjunto: a credencial OAuth, que
// vive no cofre da plataforma (chaveiro no macOS, arquivo no Windows), e o
// bloco de identidade gravado em ~/.claude.json. Mover só a primeira deixa a
// CLI autenticada como uma conta e exibindo outra — por isso o domínio trata
// as duas como uma unidade indivisível, a Session.
package ccaccount

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
)

// Erros do domínio. São sentinelas porque a tela decide o texto e a ação a
// partir deles — de "pedir confirmação" a "mandar rodar `claude`".
var (
	ErrNoActiveSession = errors.New("nenhuma sessão ativa do Claude Code — rode `claude` e faça login")
	ErrNoDirectAuth    = errors.New("nenhuma autenticação direta configurada")
	ErrProfileNotFound = errors.New("perfil não encontrado")
	ErrProfileExists   = errors.New("já existe um perfil com esse nome, de outra conta ou autenticação")
	ErrActiveUnsaved   = errors.New("a sessão ativa não está guardada em nenhum perfil")
	ErrEmptyProfile    = errors.New("o perfil guardado não tem credencial")
	ErrInvalidName     = errors.New("nome inválido: letras, números, espaço, hífen, ponto ou sublinhado, até 40 caracteres")
)

// NameLimit é o tamanho máximo do nome de um perfil. O nome vira chave no
// cofre da plataforma, então é curto de propósito.
const NameLimit = 40

// AuthMethod identifica de onde o Claude Code obtém a credencial efetiva.
// O valor vazio é deliberadamente o login tradicional, mantendo compatíveis
// os perfis gravados antes de existirem outros métodos.
type AuthMethod string

const (
	AuthClaudeLogin AuthMethod = ""
	AuthOAuthToken  AuthMethod = "claude_code_oauth_token"
	AuthAPIKey      AuthMethod = "anthropic_api_key"
	AuthBearerToken AuthMethod = "anthropic_auth_token"
	AuthAPIHelper   AuthMethod = "api_key_helper"
)

// Label é a descrição segura do método; nunca contém o valor secreto.
func (m AuthMethod) Label() string {
	switch m {
	case AuthOAuthToken:
		return "token OAuth"
	case AuthAPIKey:
		return "API key"
	case AuthBearerToken:
		return "token Bearer"
	case AuthAPIHelper:
		return "apiKeyHelper"
	default:
		return "login do Claude"
	}
}

// Session é o estado completo de uma conta: tudo o que a CLI precisa
// encontrar para se considerar logada naquela conta, e nada além disso.
//
// A fronteira é deliberada. Entra o que identifica, autentica e configura a
// conta; fica de fora o que é trabalho — transcrições, planos, tarefas,
// histórico —, que continua no lugar para que trocar de conta não faça o
// usuário perder de onde parou.
type Session struct {
	// Method vazio representa o login persistido pela própria CLI. Nos
	// demais métodos, AuthValue é o token, a chave ou o comando do helper.
	// Ambos são derivados de Settings: descrevem o que vence, não são a
	// fonte da verdade da restauração.
	Method    AuthMethod
	AuthValue string
	// Origin só descreve a fonte ativa na tela; nunca é persistida no perfil.
	Origin string
	// Credential é o JSON cru que o Claude Code guarda no cofre. Guardamos
	// o blob inteiro, e não os campos que sabemos ler, porque ele carrega o
	// refresh token — sem ele a sessão restaurada morreria em uma hora.
	Credential json.RawMessage
	// Account é o valor cru de "oauthAccount" em ~/.claude.json.
	Account json.RawMessage
	// UserID é o campo "userID" do mesmo arquivo.
	UserID string
	// Caches são as chaves do ~/.claude.json cujo conteúdo pertence à conta:
	// cotas, modelos liberados, elegibilidade de plano. Guardá-las com o
	// perfil é o que faz a conta voltar como estava, e não como se fosse a
	// primeira vez.
	Caches map[string]json.RawMessage
	// Settings é o ~/.claude/settings.json inteiro. Inteiro, e não uma lista
	// de chaves conhecidas: foi exatamente uma lista dessas que deixou um
	// ANTHROPIC_BASE_URL de gateway apontado para o lugar errado depois de
	// voltar para uma conta de login comum.
	Settings json.RawMessage
}

// Valid informa se a sessão tem o mínimo para ser restaurada.
//
// Credencial ou autenticação direta bastam: um perfil de API key não tem
// credencial no cofre, e um de login não precisa de token no settings.
func (s Session) Valid() bool {
	return len(s.Credential) > 0 || s.AuthValue != ""
}

// Identity são os campos legíveis de uma conta, extraídos da sessão.
type Identity struct {
	Email        string
	DisplayName  string
	Organization string
	AccountUUID  string
	// Plan é o subscriptionType da credencial: "pro", "max", "team"…
	Plan string
	// ExpiresAt é a validade do access token. Vencido não é problema: a CLI
	// renova sozinha no próximo uso.
	ExpiresAt time.Time
	// RenewsUntil é a validade do refresh token. Vencido esse, só um novo
	// login resolve.
	RenewsUntil time.Time
}

// Label é o melhor nome disponível para mostrar na tela.
func (i Identity) Label() string {
	switch {
	case i.Email != "":
		return i.Email
	case i.DisplayName != "":
		return i.DisplayName
	case i.AccountUUID != "":
		return i.AccountUUID
	default:
		return "conta desconhecida"
	}
}

// Empty informa se nada foi identificado.
func (i Identity) Empty() bool {
	return i.Email == "" && i.DisplayName == "" && i.AccountUUID == "" && i.Plan == ""
}

// Stale informa que o access token venceu — a CLI renova sozinha.
func (i Identity) Stale(now time.Time) bool {
	return !i.ExpiresAt.IsZero() && !i.ExpiresAt.After(now)
}

// Dead informa que nem o refresh token vale mais: só um login novo resolve.
func (i Identity) Dead(now time.Time) bool {
	return !i.RenewsUntil.IsZero() && !i.RenewsUntil.After(now)
}

// SameAccount compara duas identidades pelo que de fato as identifica.
//
// O UUID é a chave; o e-mail entra como reserva porque uma sessão lida antes
// da CLI popular o ~/.claude.json não tem UUID nenhum.
func (i Identity) SameAccount(other Identity) bool {
	if i.AccountUUID != "" && other.AccountUUID != "" {
		return i.AccountUUID == other.AccountUUID
	}
	if i.Email != "" && other.Email != "" {
		return strings.EqualFold(i.Email, other.Email)
	}
	return false
}

// Profile é uma sessão guardada sob um nome.
type Profile struct {
	Name     string
	Method   AuthMethod
	Identity Identity
	SavedAt  time.Time
}

// --- Portas de saída ---------------------------------------------------

// Vault é o cofre onde a CLI guarda a credencial ativa.
type Vault interface {
	Read(ctx context.Context) (json.RawMessage, error)
	Write(ctx context.Context, credential json.RawMessage) error
	// Origin descreve para o usuário onde essa credencial mora.
	Origin() string
}

// Config é o ~/.claude.json, do qual só o que pertence à conta nos
// interessa: a identidade e os caches escopados nela. Todo o resto do
// arquivo — projetos, dicas, contadores, machineID — é da máquina e do
// trabalho, e atravessa a troca intacto.
type Config interface {
	Account(ctx context.Context) (account json.RawMessage, userID string, caches map[string]json.RawMessage, err error)
	SetAccount(ctx context.Context, account json.RawMessage, userID string, caches map[string]json.RawMessage) error
}

// Settings é o ~/.claude/settings.json, capturado e devolvido inteiro.
type Settings interface {
	// Snapshot devolve o arquivo como está. Ausente devolve nil sem erro:
	// uma conta sem settings.json é uma conta legítima.
	Snapshot(ctx context.Context) (json.RawMessage, error)
	// Restore grava o arquivo inteiro.
	Restore(ctx context.Context, doc json.RawMessage) error
	// ApplyAuth troca somente a autenticação, preservando o resto do
	// arquivo. É o caminho dos perfis salvos antes de a tool passar a
	// guardar o settings inteiro: restaurar um snapshot que eles não têm
	// apagaria model, tema e permissões de quem só queria trocar de conta.
	ApplyAuth(ctx context.Context, method AuthMethod, value string) error
	// Auth descreve a autenticação efetiva na ordem de precedência da CLI,
	// considerando também o que o shell exportou.
	Auth(ctx context.Context) (method AuthMethod, value, origin string, err error)
}

// Store é onde o lealing guarda os perfis.
type Store interface {
	List(ctx context.Context) ([]Profile, error)
	Load(ctx context.Context, name string) (Session, error)
	Save(ctx context.Context, profile Profile, session Session) error
	Delete(ctx context.Context, name string) error
}

// Switcher é a porta de entrada da tool: tudo o que a tela pode pedir.
// Manter o contrato explícito é o que permite testar a tela com um duplo,
// sem chaveiro nem ~/.claude.json por perto.
type Switcher interface {
	State(ctx context.Context) (State, error)
	Save(ctx context.Context, name string) (Profile, error)
	SaveOverwriting(ctx context.Context, name string) (Profile, error)
	Activate(ctx context.Context, name string) error
	ActivateOverwriting(ctx context.Context, name string) error
	Forget(ctx context.Context, name string) error
}

// --- Serviço -----------------------------------------------------------

// Manager é o caso de uso da tool: ler o estado, guardar a sessão ativa e
// restaurar uma guardada.
type Manager struct {
	vault    Vault
	config   Config
	store    Store
	settings Settings
	now      func() time.Time
}

// WithSettings habilita a captura e a restauração do settings.json —
// e com ela tokens, API keys, apiKeyHelper e tudo o mais que o arquivo
// carrega. Fica opcional para preservar o construtor compatível com
// integrações que só conhecem o login tradicional.
func (m *Manager) WithSettings(settings Settings) *Manager {
	m.settings = settings
	return m
}

var _ Switcher = (*Manager)(nil)

// NewManager monta o serviço.
func NewManager(vault Vault, config Config, store Store, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{vault: vault, config: config, store: store, now: now}
}

// State é o retrato que a tela desenha.
type State struct {
	// Active é a conta em que a CLI está agora; vazia se não houver sessão.
	Active    Identity
	HasActive bool
	// ActiveProfile é o nome do perfil que corresponde à sessão ativa, ou
	// vazio se ela não está guardada em lugar nenhum.
	ActiveProfile string
	Method        AuthMethod
	// Origin é onde a credencial ativa mora, para a tela dizer ao usuário.
	Origin   string
	Profiles []Profile
}

// State lê o estado atual. Falta de sessão ativa não é erro: é o retrato de
// quem ainda não fez login, e a tela precisa desenhá-lo.
func (m *Manager) State(ctx context.Context) (State, error) {
	profiles, err := m.store.List(ctx)
	if err != nil {
		return State{}, err
	}

	st := State{Profiles: profiles}

	session, err := m.Current(ctx)
	st.Origin = m.vault.Origin()
	if errors.Is(err, ErrNoActiveSession) {
		return st, nil
	}
	if err != nil {
		return st, err
	}

	st.Active = Describe(session)
	st.HasActive = true
	st.Method = session.Method
	if session.Origin != "" {
		st.Origin = session.Origin
	}
	if idx, ok := m.activeProfile(ctx, profiles, session); ok {
		profile, err := m.syncProfile(ctx, profiles[idx], session)
		if err != nil {
			return st, fmt.Errorf("atualizar a credencial rotacionada do perfil ativo: %w", err)
		}
		st.Profiles[idx] = profile
		st.Active = mergeIdentity(profile.Identity, st.Active)
		st.ActiveProfile = profile.Name
	}
	return st, nil
}

// Current lê a sessão ativa juntando cofre, configuração e settings.
//
// Lê tudo, e não só a fonte que vence: uma conta com API key no settings
// pode ter também o login OAuth no cofre, e um perfil que guardasse apenas
// a primeira voltaria pela metade. Method descreve qual delas a CLI vai
// usar; a sessão carrega as duas.
func (m *Manager) Current(ctx context.Context) (Session, error) {
	var session Session

	if m.settings != nil {
		doc, err := m.settings.Snapshot(ctx)
		if err != nil {
			return Session{}, err
		}
		session.Settings = doc

		method, value, origin, err := m.settings.Auth(ctx)
		switch {
		case err == nil:
			session.Method, session.AuthValue, session.Origin = method, value, origin
		case !errors.Is(err, ErrNoDirectAuth):
			return Session{}, err
		}
	}

	if cred, err := m.vault.Read(ctx); err == nil && len(cred) > 0 {
		session.Credential = cred
	}
	if !session.Valid() {
		return Session{}, ErrNoActiveSession
	}

	// A identidade é acessório: uma credencial sem ~/.claude.json ainda é
	// uma sessão válida, só menos informativa.
	account, userID, caches, _ := m.config.Account(ctx)
	session.Account, session.UserID, session.Caches = account, userID, caches
	return session, nil
}

// Save guarda a sessão ativa sob um nome.
//
// Devolve ErrProfileExists quando o nome já pertence a outra conta:
// sobrescrever ali apagaria a única cópia de uma credencial que o usuário
// não tem como recuperar sem refazer o login.
func (m *Manager) Save(ctx context.Context, name string) (Profile, error) {
	return m.save(ctx, name, false)
}

// SaveOverwriting é o Save depois de o usuário confirmar a substituição.
func (m *Manager) SaveOverwriting(ctx context.Context, name string) (Profile, error) {
	return m.save(ctx, name, true)
}

func (m *Manager) save(ctx context.Context, name string, force bool) (Profile, error) {
	clean, err := NormalizeName(name)
	if err != nil {
		return Profile{}, err
	}

	session, err := m.Current(ctx)
	if err != nil {
		return Profile{}, err
	}
	identity := Describe(session)

	if !force {
		existing, err := m.store.List(ctx)
		if err != nil {
			return Profile{}, err
		}
		for _, p := range existing {
			if strings.EqualFold(p.Name, clean) {
				saved, loadErr := m.store.Load(ctx, p.Name)
				if loadErr != nil || !sameSession(saved, session) && !p.Identity.SameAccount(identity) {
					return Profile{}, ErrProfileExists
				}
			}
		}
	}

	profile := Profile{Name: clean, Method: session.Method, Identity: identity, SavedAt: m.now()}
	if err := m.store.Save(ctx, profile, session); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// Activate devolve a sessão do perfil ao cofre e ao ~/.claude.json.
//
// Devolve ErrActiveUnsaved quando a sessão que seria sobrescrita não está
// guardada em nenhum perfil — é a única proteção possível, porque depois da
// escrita a credencial anterior não existe mais em lugar nenhum.
func (m *Manager) Activate(ctx context.Context, name string) error {
	return m.activate(ctx, name, false)
}

// ActivateOverwriting é o Activate depois de o usuário confirmar a perda.
func (m *Manager) ActivateOverwriting(ctx context.Context, name string) error {
	return m.activate(ctx, name, true)
}

func (m *Manager) activate(ctx context.Context, name string, force bool) error {
	profiles, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	target, ok := findProfile(profiles, name)
	if !ok {
		return ErrProfileNotFound
	}

	current, currentErr := m.Current(ctx)
	hasCurrent := currentErr == nil
	if hasCurrent {
		if idx, saved := m.activeProfile(ctx, profiles, current); saved {
			// O Claude Code rotaciona o refresh token enquanto a conta está
			// em uso. Persistir a sessão mais recente imediatamente antes da
			// troca impede que voltar ao perfil restaure um token revogado.
			updated, err := m.syncProfile(ctx, profiles[idx], current)
			if err != nil {
				return fmt.Errorf("preservar a sessão atual antes da troca: %w", err)
			}
			profiles[idx] = updated
			if strings.EqualFold(updated.Name, target.Name) {
				return nil
			}
		} else if !force {
			return ErrActiveUnsaved
		}
	}

	// Carregar depois de sincronizar é importante quando o alvo é também o
	// perfil ativo: a versão antiga pode conter um refresh token já rotacionado.
	session, err := m.store.Load(ctx, target.Name)
	if err != nil {
		return err
	}
	if !session.Valid() {
		return ErrEmptyProfile
	}

	if err := m.install(ctx, session); err != nil {
		return m.activationFailure(ctx, current, hasCurrent,
			fmt.Errorf("aplicar a autenticação do perfil: %w", err))
	}

	// Não confie apenas no sucesso das escritas. Cofres de sistema podem
	// aceitar a operação e devolver outro item; reler fecha esse caso antes
	// de anunciar na UI que a troca terminou.
	installed, err := m.Current(ctx)
	if err != nil {
		return m.activationFailure(ctx, current, hasCurrent,
			fmt.Errorf("conferir a sessão depois da troca: %w", err))
	}
	if !installedAs(session, installed) {
		return m.activationFailure(ctx, current, hasCurrent,
			errors.New("a sessão conferida depois da troca difere do perfil escolhido"))
	}
	return nil
}

// install devolve a conta ao lugar, na ordem que a precedência do Claude
// Code exige: a credencial fica pronta no cofre antes de o settings.json
// decidir o que a CLI vai olhar primeiro.
func (m *Manager) install(ctx context.Context, session Session) error {
	if len(session.Credential) > 0 {
		if err := m.vault.Write(ctx, session.Credential); err != nil {
			return fmt.Errorf("gravar a credencial do login: %w", err)
		}
	}
	if err := m.config.SetAccount(ctx, session.Account, session.UserID, session.Caches); err != nil {
		return fmt.Errorf("gravar a identidade da conta: %w", err)
	}

	if m.settings == nil {
		if len(session.Settings) > 0 || session.AuthValue != "" {
			return errors.New("esta instalação da tool não gerencia o settings.json")
		}
		return nil
	}
	// Perfil com snapshot volta inteiro. Sem snapshot é um perfil salvo por
	// uma versão anterior da tool: ali só a autenticação foi guardada, e
	// escrever um arquivo que não temos apagaria as preferências do usuário.
	if len(session.Settings) > 0 {
		if err := m.settings.Restore(ctx, session.Settings); err != nil {
			return fmt.Errorf("restaurar as preferências da conta: %w", err)
		}
		return nil
	}
	if err := m.settings.ApplyAuth(ctx, session.Method, session.AuthValue); err != nil {
		return fmt.Errorf("aplicar a autenticação do perfil: %w", err)
	}
	return nil
}

// activationFailure tenta devolver a conta anterior ao lugar quando uma
// troca parcial falha. Sem sessão anterior não há credencial que possa ser
// reconstruída, então o erro original é preservado.
func (m *Manager) activationFailure(ctx context.Context, previous Session, hadPrevious bool, cause error) error {
	if !hadPrevious {
		return cause
	}
	// O contexto da operação pode ter vencido justamente depois de uma
	// escrita parcial. A restauração ganha uma janela própria e curta; usar o
	// contexto já cancelado faria o rollback falhar antes de começar.
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if rollbackErr := m.install(rollbackCtx, previous); rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("também não foi possível restaurar a sessão anterior: %w", rollbackErr))
	}
	return cause
}

// activeProfile encontra qual perfil cobre a sessão atual. A credencial exata
// vem primeiro: no Windows, especialmente em logins feitos pela extensão, o
// oauthAccount do ~/.claude.json pode continuar descrevendo a conta anterior.
// Depois que o token rotaciona, a identidade entra como reserva, mas somente
// quando aponta sem ambiguidade para um único perfil.
func (m *Manager) activeProfile(ctx context.Context, profiles []Profile, session Session) (int, bool) {
	for idx := range profiles {
		saved, err := m.store.Load(ctx, profiles[idx].Name)
		if err == nil && sameAuth(saved, session) {
			return idx, true
		}
	}

	id := Describe(session)
	if id.AccountUUID == "" && id.Email == "" {
		return 0, false
	}
	match, matches := 0, 0
	for idx, profile := range profiles {
		if profile.Identity.SameAccount(id) {
			match, matches = idx, matches+1
		}
	}
	return match, matches == 1
}

// syncProfile atualiza silenciosamente a fotografia da conta ativa quando o
// Claude Code rotacionou seus tokens. Sem isso, sair e voltar ao perfil
// restaura um refresh token que o servidor já revogou.
func (m *Manager) syncProfile(ctx context.Context, profile Profile, current Session) (Profile, error) {
	saved, err := m.store.Load(ctx, profile.Name)
	if err != nil {
		return Profile{}, err
	}
	// A extensão no Windows pode trocar a credencial sem atualizar (ou até
	// sem criar) oauthAccount. Quando a credencial identifica exatamente o
	// perfil, não deixe essa identidade ausente/antiga contaminar a cópia boa
	// que já estava guardada.
	expected := mergeIdentity(profile.Identity, Describe(saved))
	observed := Describe(current)
	if expected.AccountUUID != "" || expected.Email != "" {
		if observed.AccountUUID == "" && observed.Email == "" || !expected.SameAccount(observed) {
			// Os caches acompanham a identidade: se o bloco descreve a conta
			// errada, as cotas e os modelos ao lado também descrevem.
			current.Account = saved.Account
			current.UserID = saved.UserID
			current.Caches = saved.Caches
		}
	}
	if sameSession(saved, current) {
		return profile, nil
	}
	// Um perfil salvo antes de a tool guardar o settings ganha o snapshot
	// aqui, sozinho, na primeira vez que a conta dele estiver ativa.
	profile.Method = current.Method
	profile.Identity = mergeIdentity(profile.Identity, Describe(current))
	profile.SavedAt = m.now()
	if err := m.store.Save(ctx, profile, current); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// sameSession informa se o perfil guardado já descreve a sessão observada.
//
// Os caches ficam de fora de propósito: a CLI os reescreve o tempo todo, e
// incluí-los faria a tela regravar o perfil a cada atualização — no macOS,
// um processo do chaveiro por perfil por refresh.
func sameSession(a, b Session) bool {
	return sameAuth(a, b) &&
		sameJSON(a.Account, b.Account) && a.UserID == b.UserID &&
		sameJSON(a.Settings, b.Settings)
}

// sameAuth compara só o que autentica. É o que casa perfil e sessão ativa,
// e por isso ignora o settings: mexer no tema ou num hook não pode fazer a
// conta ativa parecer não guardada.
func sameAuth(a, b Session) bool {
	if a.Method != b.Method || a.AuthValue != b.AuthValue {
		return false
	}
	// No login tradicional a credencial é a própria autenticação. Nos métodos
	// diretos ela é bagagem: um perfil guardado sem credencial não deixa de
	// ser o mesmo porque o cofre tem uma sobrando de outro login.
	if a.Method != AuthClaudeLogin && (len(a.Credential) == 0 || len(b.Credential) == 0) {
		return true
	}
	return sameJSON(a.Credential, b.Credential)
}

// installedAs confere a sessão relida contra o perfil que acabou de ser
// aplicado. Um perfil sem snapshot de settings não promete nada sobre o
// arquivo — cobrar dele o que não guardou reprovaria uma troca correta.
//
// O settings é conferido por continência, não byte a byte: restaurar o
// snapshot reescreve o arquivo pelo encoder da plataforma, que reordena as
// chaves do mapa e ainda pode neutralizar com valor vazio uma autenticação
// que só o shell exportava. Exigir bytes idênticos reprovava uma troca
// correta só porque o arquivo relido tinha as mesmas chaves em outra ordem.
func installedAs(want, got Session) bool {
	if !sameAuth(want, got) || !sameJSON(want.Account, got.Account) || want.UserID != got.UserID {
		return false
	}
	return len(want.Settings) == 0 || jsonContains(want.Settings, got.Settings)
}

// jsonContains informa se tudo o que "want" declara reaparece em "got",
// ignorando a ordem das chaves e tolerando chaves a mais em "got". É a
// conferência certa para o settings restaurado: o perfil promete o que
// guardou, não que o arquivo seja idêntico byte a byte depois de reescrito.
func jsonContains(want, got json.RawMessage) bool {
	if len(want) == 0 {
		return true
	}
	var wantValue, gotValue any
	if json.Unmarshal(want, &wantValue) != nil || json.Unmarshal(got, &gotValue) != nil {
		return sameJSON(want, got)
	}
	return valueContains(wantValue, gotValue)
}

// valueContains é a continência recursiva: mapas exigem cada chave de "want"
// presente e contida em "got"; listas e escalares exigem igualdade.
func valueContains(want, got any) bool {
	switch wantTyped := want.(type) {
	case map[string]any:
		gotMap, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for key, wantField := range wantTyped {
			gotField, ok := gotMap[key]
			if !ok || !valueContains(wantField, gotField) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(want, got)
	}
}

func sameJSON(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var compactA, compactB bytes.Buffer
	if json.Compact(&compactA, a) != nil || json.Compact(&compactB, b) != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(compactA.Bytes(), compactB.Bytes())
}

// mergeIdentity conserva metadados que uma leitura parcial do arquivo da
// CLI não trouxe e substitui somente os campos efetivamente observados.
func mergeIdentity(old, fresh Identity) Identity {
	if fresh.Email != "" {
		old.Email = fresh.Email
	}
	if fresh.DisplayName != "" {
		old.DisplayName = fresh.DisplayName
	}
	if fresh.Organization != "" {
		old.Organization = fresh.Organization
	}
	if fresh.AccountUUID != "" {
		old.AccountUUID = fresh.AccountUUID
	}
	if fresh.Plan != "" {
		old.Plan = fresh.Plan
	}
	if !fresh.ExpiresAt.IsZero() {
		old.ExpiresAt = fresh.ExpiresAt
	}
	if !fresh.RenewsUntil.IsZero() {
		old.RenewsUntil = fresh.RenewsUntil
	}
	return old
}

// Forget apaga um perfil. A sessão ativa não é tocada: esquecer o perfil da
// conta em uso não desloga ninguém.
func (m *Manager) Forget(ctx context.Context, name string) error {
	profiles, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	target, ok := findProfile(profiles, name)
	if !ok {
		return ErrProfileNotFound
	}
	return m.store.Delete(ctx, target.Name)
}

// --- Regras puras ------------------------------------------------------

// NormalizeName valida e limpa o nome de um perfil.
func NormalizeName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" || len([]rune(clean)) > NameLimit {
		return "", ErrInvalidName
	}
	for _, r := range clean {
		if !AllowedInName(r) {
			return "", ErrInvalidName
		}
	}
	return clean, nil
}

// AllowedInName informa se um caractere pode entrar no nome de um perfil.
//
// O conjunto é restrito porque o nome vira chave no cofre da plataforma e
// nome de conta no chaveiro do macOS: barras, dois-pontos e caracteres de
// controle não têm o mesmo significado nos dois lados.
func AllowedInName(r rune) bool {
	switch {
	case unicode.IsLetter(r), unicode.IsDigit(r):
		return true
	case r == ' ', r == '-', r == '_', r == '.':
		return true
	default:
		return false
	}
}

// Describe extrai a identidade dos dois blobs de uma sessão.
//
// Blob ilegível vira campo vazio, nunca erro: uma tela que se recusa a abrir
// porque a CLI renomeou um campo é pior que uma tela com um traço.
func Describe(s Session) Identity {
	if s.Method != AuthClaudeLogin {
		return Identity{DisplayName: s.Method.Label()}
	}
	var id Identity

	var cred struct {
		OAuth struct {
			ExpiresAt             int64  `json:"expiresAt"`
			RefreshTokenExpiresAt int64  `json:"refreshTokenExpiresAt"`
			SubscriptionType      string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(s.Credential, &cred); err == nil {
		id.Plan = cred.OAuth.SubscriptionType
		// As duas datas vêm em milissegundos desde a época.
		if ms := cred.OAuth.ExpiresAt; ms > 0 {
			id.ExpiresAt = time.UnixMilli(ms)
		}
		if ms := cred.OAuth.RefreshTokenExpiresAt; ms > 0 {
			id.RenewsUntil = time.UnixMilli(ms)
		}
	}

	var account struct {
		EmailAddress     string `json:"emailAddress"`
		DisplayName      string `json:"displayName"`
		OrganizationName string `json:"organizationName"`
		AccountUUID      string `json:"accountUuid"`
	}
	if err := json.Unmarshal(s.Account, &account); err == nil {
		id.Email = account.EmailAddress
		id.DisplayName = account.DisplayName
		id.Organization = account.OrganizationName
		id.AccountUUID = account.AccountUUID
	}
	return id
}

// findProfile procura pelo nome, ignorando maiúsculas.
func findProfile(profiles []Profile, name string) (Profile, bool) {
	for _, p := range profiles {
		if strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			return p, true
		}
	}
	return Profile{}, false
}

// matchProfile procura o perfil que guarda a conta descrita.
func matchProfile(profiles []Profile, id Identity) (Profile, bool) {
	for _, p := range profiles {
		if p.Identity.SameAccount(id) {
			return p, true
		}
	}
	return Profile{}, false
}
