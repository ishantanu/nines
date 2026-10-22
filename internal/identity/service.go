package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ishantanu/nines/internal/tenancy"
)

type User struct {
	Version  int64  `json:"version"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Epoch    int64  `json:"sessionEpoch"`
}
type Member struct {
	UserID        string `json:"userId"`
	Role          string `json:"role"`
	EffectiveRole string `json:"effectiveRole"`
	Version       int64  `json:"version"`
}
type MemberView struct {
	Member
	User User `json:"user"`
}
type Team struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Role    string   `json:"role"`
	Members []string `json:"members"`
	Version int64    `json:"version"`
}
type Account struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tenant    string    `json:"tenant"`
	Role      string    `json:"role"`
	Expires   time.Time `json:"expiresAt"`
	Created   time.Time `json:"createdAt"`
	TokenHash string    `json:"tokenHash"`
}
type AccountView struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Role    string    `json:"role"`
	Expires time.Time `json:"expiresAt"`
	Created time.Time `json:"createdAt"`
}

func (a Account) Public() AccountView { return AccountView{a.ID, a.Name, a.Role, a.Expires, a.Created} }

type Session struct {
	UserID  string    `json:"userId"`
	Epoch   int64     `json:"epoch"`
	Expires time.Time `json:"expiresAt"`
}
type flow struct {
	Provider string    `json:"provider"`
	Nonce    string    `json:"nonce"`
	Verifier string    `json:"verifier"`
	Expires  time.Time `json:"expiresAt"`
}
type Actor struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Provider        string            `json:"provider,omitempty"`
	IdentitySubject string            `json:"identitySubject,omitempty"`
	Grants          map[string]string `json:"-"`
	Session         bool              `json:"-"`
	Expires         time.Time         `json:"expiresAt,omitempty"`
}
type Audit struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	ActorID   string    `json:"actorId"`
	ActorKind string    `json:"actorKind"`
	ActorName string    `json:"actorName"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Outcome   string    `json:"outcome"`
	RequestID string    `json:"requestId,omitempty"`
	Revision  string    `json:"revision,omitempty"`
}
type ProviderView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}
type limitEntry struct {
	limiter *rate.Limiter
	at      time.Time
}
type Service struct {
	repo       Repository
	config     Config
	providers  map[string]provider
	tenants    map[string]bool
	Now        func() time.Time
	limitMu    sync.Mutex
	limits     map[string]limitEntry
	loginSlots chan struct{}
}

func New(ctx context.Context, c Config, repo Repository, tenants map[string]bool) (*Service, error) {
	s := &Service{repo: repo, config: c, providers: map[string]provider{}, tenants: tenants, Now: time.Now, limits: map[string]limitEntry{}, loginSlots: make(chan struct{}, 8)}
	for _, cfg := range c.Providers {
		if err := validateMappings(&cfg); err != nil {
			return nil, err
		}
		for _, m := range cfg.RoleMappings {
			if !tenants[m.Tenant] {
				return nil, fmt.Errorf("SSO mapping references unconfigured tenant %s", m.Tenant)
			}
		}
		p, err := initProvider(ctx, cfg, c.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", cfg.ID, err)
		}
		s.providers[cfg.ID] = p
	}
	grouped := map[string][]Bootstrap{}
	for _, b := range c.Bootstrap {
		if !tenants[b.Tenant] {
			return nil, fmt.Errorf("bootstrap references unconfigured tenant")
		}
		grouped[b.Tenant] = append(grouped[b.Tenant], b)
	}
	for tenant, entries := range grouped {
		if err := repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
			if _, err := tx.Get("bootstrap", tenant, "complete"); err == nil {
				return nil
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			for _, b := range entries {
				u := s.user(b.Provider, externalUser{Subject: b.Subject})
				existing, err := read[User](tx, "user", "global", u.ID)
				if err == nil {
					u = existing
				} else if !errors.Is(err, ErrNotFound) {
					return err
				}
				b, _ := json.Marshal(u)
				if err = tx.Ensure("user", "global", u.ID, b); err != nil {
					return err
				}
				if err = s.saveMember(tx, tenant, Member{u.ID, "owner", "owner", 1}); err != nil {
					return err
				}
			}
			return put(tx, "bootstrap", tenant, "complete", map[string]bool{"complete": true}, time.Time{})
		}); err != nil {
			return nil, err
		}
	}
	if err := s.bootstrapLocal(ctx); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Service) Close() { s.repo.Close() }
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(15 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			job, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = s.repo.Prune(job)
			cancel()
		}
	}
}
func (s *Service) Providers() []ProviderView {
	out := []ProviderView{}
	for _, p := range s.config.Providers {
		out = append(out, ProviderView{p.ID, p.Name, p.Type})
	}
	return out
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("system random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) user(id string, e externalUser) User {
	p := s.providers[id].config
	origin := p.IssuerURL
	if p.Type == "ldap" {
		origin = p.URL + "|" + p.UserBaseDN
	}
	name := e.Name
	if name == "" {
		name = e.Subject
	}
	return User{ID: "u-" + hash(id + "\x00" + origin + "\x00" + e.Subject)[:40], Provider: id, Subject: e.Subject, Name: name, Email: e.Email}
}
func (s *Service) login(ctx context.Context, id string, e externalUser) (string, Actor, error) {
	if e.Subject == "" || len(e.Subject) > 500 || len(e.Name) > 300 || len(e.Email) > 320 {
		return "", Actor{}, ErrForbidden
	}
	u := s.user(id, e)
	token := randomToken()
	expires := s.Now().Add(time.Duration(s.config.SessionHours) * time.Hour)
	err := s.repo.Update(ctx, "user:"+u.ID, func(tx Tx) error {
		if e.LocalCredentialDigest != "" {
			credential, err := read[localCredential](tx, "local_password", "global", u.ID)
			if err != nil || credential.Disabled || hash(string(credential.Hash)) != e.LocalCredentialDigest {
				return ErrForbidden
			}
		}
		existing, err := read[User](tx, "user", "global", u.ID)
		if err == nil {
			u.Epoch = existing.Epoch
			u.Version = existing.Version
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err = put(tx, "user", "global", u.ID, u, time.Time{}); err != nil {
			return err
		}
		if err = s.saveAuthorization(tx, u, e.Authorization, &expires); err != nil {
			return err
		}
		return put(tx, "session", "global", hash(token), Session{u.ID, u.Epoch, expires}, expires)
	})
	if err != nil {
		return "", Actor{}, err
	}
	actor, err := s.SessionActor(ctx, token)
	return token, actor, err
}
func (s *Service) SessionActor(ctx context.Context, token string) (Actor, error) {
	var actor Actor
	if len(token) != 43 {
		return actor, ErrForbidden
	}
	e := s.repo.View(ctx, func(tx Tx) error { var e error; actor, e = s.sessionActorTx(tx, hash(token)); return e })
	return actor, e
}
func (s *Service) sessionActorTx(tx Tx, h string) (Actor, error) {
	v, e := read[Session](tx, "session", "global", h)
	if e != nil {
		return Actor{}, e
	}
	if !v.Expires.After(s.Now()) {
		return Actor{}, ErrForbidden
	}
	u, e := read[User](tx, "user", "global", v.UserID)
	if e != nil {
		return Actor{}, e
	}
	if u.Epoch != v.Epoch {
		return Actor{}, ErrForbidden
	}
	g, e := s.effectiveGrants(tx, u)
	if e != nil {
		return Actor{}, e
	}
	return Actor{ID: u.ID, Kind: "user", Name: u.Name, Provider: u.Provider, IdentitySubject: u.Subject, Grants: g, Session: true, Expires: v.Expires}, nil
}

type grant struct {
	Tenant string `json:"tenant"`
	Member Member `json:"member"`
}

func (s *Service) saveMember(tx Tx, tenant string, m Member) error {
	if err := put(tx, "member", tenant, m.UserID, m, time.Time{}); err != nil {
		return err
	}
	return put(tx, "grant", m.UserID, tenant, grant{tenant, m}, time.Time{})
}
func (s *Service) ServiceActor(ctx context.Context, token string) (Actor, error) {
	var actor Actor
	err := s.repo.View(ctx, func(tx Tx) error {
		a, err := read[Account](tx, "service_token", "global", hash(token))
		if err != nil {
			return err
		}
		if !s.tenants[a.Tenant] || !a.Expires.After(s.Now()) || !validRole(a.Role, false) {
			return ErrForbidden
		}
		actor = Actor{ID: a.ID, Kind: "service_account", Name: a.Name, Grants: map[string]string{a.Tenant: a.Role}, Expires: a.Expires}
		return nil
	})
	return actor, err
}
func rank(role string) int {
	return map[string]int{"viewer": 1, "agent": 2, "editor": 3, "owner": 4}[role]
}
func validRole(role string, owner bool) bool { return rank(role) > 0 && (owner || role != "owner") }
func (s *Service) checkOwner(tx Tx, tenant string, a Actor) error {
	if !s.tenants[tenant] {
		return ErrForbidden
	}
	if a.Kind == "api_token" && a.Grants[tenant] == "owner" {
		return nil
	}
	if a.Kind != "user" {
		return ErrForbidden
	}
	m, err := read[Member](tx, "member", tenant, a.ID)
	if err == nil && m.EffectiveRole == "owner" {
		return nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	u, err := read[User](tx, "user", "global", a.ID)
	if err != nil {
		return ErrForbidden
	}
	mapped, _, err := s.mappedGrants(tx, u)
	if err != nil {
		return err
	}
	if mapped[tenant] != "owner" {
		return ErrForbidden
	}
	return nil
}
func (s *Service) recompute(tx Tx, tenant, user string) error {
	m, err := read[Member](tx, "member", tenant, user)
	if err != nil {
		return err
	}
	m.EffectiveRole = m.Role
	teams, err := list[Team](tx, "team", tenant)
	if err != nil {
		return err
	}
	for _, t := range teams {
		for _, id := range t.Members {
			if id == user && rank(t.Role) > rank(m.EffectiveRole) {
				m.EffectiveRole = t.Role
			}
		}
	}
	m.Version++
	return s.saveMember(tx, tenant, m)
}
func (s *Service) Grant(ctx context.Context, a Actor, tenant, providerID, subject, role string, expected int64) (Member, error) {
	var result Member
	_, ok := s.providers[providerID]
	if !ok || subject == "" || len(subject) > 500 || !validRole(role, true) || expected < 0 {
		return result, ErrInvalid
	}
	u := s.user(providerID, externalUser{Subject: subject})
	err := s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		m, err := read[Member](tx, "member", tenant, u.ID)
		if errors.Is(err, ErrNotFound) {
			if expected != 0 {
				return ErrConflict
			}
			all, e := tx.List("member", tenant)
			if e != nil {
				return e
			}
			if len(all) >= maxRows {
				return ErrInvalid
			}
			m = Member{UserID: u.ID, Version: 0}
		} else if err != nil {
			return err
		}
		if m.Version != expected {
			return ErrConflict
		}
		if m.Role == "owner" && role != "owner" {
			if err = s.checkOtherOwner(tx, tenant, u.ID); err != nil {
				return err
			}
		}
		existing, err := read[User](tx, "user", "global", u.ID)
		if errors.Is(err, ErrNotFound) {
			b, _ := json.Marshal(u)
			if err = tx.Ensure("user", "global", u.ID, b); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			u = existing
		}
		m.Role = role
		if err = s.saveMember(tx, tenant, m); err != nil {
			return err
		}
		if err = s.recompute(tx, tenant, u.ID); err != nil {
			return err
		}
		result, err = read[Member](tx, "member", tenant, u.ID)
		return err
	})
	return result, err
}
func (s *Service) checkOtherOwner(tx Tx, tenant, user string) error {
	members, err := list[Member](tx, "member", tenant)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.UserID != user && m.Role == "owner" {
			return nil
		}
	}
	return fmt.Errorf("cannot remove or demote the last tenant owner: %w", ErrInvalid)
}
func (s *Service) RemoveMember(ctx context.Context, a Actor, tenant, user string, expected int64) error {
	return s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		m, err := read[Member](tx, "member", tenant, user)
		if err != nil {
			return err
		}
		if expected != m.Version {
			return ErrConflict
		}
		if m.Role == "owner" {
			if err = s.checkOtherOwner(tx, tenant, user); err != nil {
				return err
			}
		}
		teams, err := list[Team](tx, "team", tenant)
		if err != nil {
			return err
		}
		for _, t := range teams {
			next := []string{}
			for _, id := range t.Members {
				if id != user {
					next = append(next, id)
				}
			}
			if len(next) != len(t.Members) {
				t.Members = next
				t.Version++
				if err = put(tx, "team", tenant, t.ID, t, time.Time{}); err != nil {
					return err
				}
			}
		}
		if err = tx.Delete("member", tenant, user); err != nil {
			return err
		}
		return tx.Delete("grant", user, tenant)
	})
}
func (s *Service) SaveTeam(ctx context.Context, a Actor, tenant string, t Team, expected int64) (Team, error) {
	if !tenancy.ValidID(t.ID) || strings.TrimSpace(t.Name) == "" || len(t.Name) > 200 || !validRole(t.Role, false) || len(t.Members) > maxRows || expected < 0 {
		return Team{}, ErrInvalid
	}
	err := s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		previous, err := read[Team](tx, "team", tenant, t.ID)
		if errors.Is(err, ErrNotFound) {
			if expected != 0 {
				return ErrConflict
			}
			all, e := tx.List("team", tenant)
			if e != nil {
				return e
			}
			if len(all) >= 100 {
				return ErrInvalid
			}
		} else if err != nil {
			return err
		}
		if previous.Version != expected {
			return ErrConflict
		}
		seen := map[string]bool{}
		for _, id := range t.Members {
			if seen[id] {
				return ErrInvalid
			}
			seen[id] = true
			if _, err = tx.Get("member", tenant, id); err != nil {
				return ErrInvalid
			}
		}
		sort.Strings(t.Members)
		if t.Members == nil {
			t.Members = []string{}
		}
		t.Version = expected + 1
		if err = put(tx, "team", tenant, t.ID, t, time.Time{}); err != nil {
			return err
		}
		for _, id := range previous.Members {
			seen[id] = true
		}
		for id := range seen {
			if err = s.recompute(tx, tenant, id); err != nil {
				return err
			}
		}
		return nil
	})
	return t, err
}
func (s *Service) RemoveTeam(ctx context.Context, a Actor, tenant, id string, expected int64) error {
	return s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		t, err := read[Team](tx, "team", tenant, id)
		if err != nil {
			return err
		}
		if t.Version != expected {
			return ErrConflict
		}
		if err = tx.Delete("team", tenant, id); err != nil {
			return err
		}
		for _, user := range t.Members {
			if err = s.recompute(tx, tenant, user); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) CreateAccount(ctx context.Context, a Actor, tenant, id, name, role string, expires time.Time) (AccountView, string, error) {
	var account Account
	if !tenancy.ValidID(id) || name == "" || len(name) > 200 || !validRole(role, false) || !expires.After(s.Now()) || expires.After(s.Now().Add(365*24*time.Hour)) {
		return account.Public(), "", ErrInvalid
	}
	token := "wp_sa_" + randomToken()
	account = Account{ID: id, Name: name, Tenant: tenant, Role: role, Expires: expires, Created: s.Now().UTC(), TokenHash: hash(token)}
	err := s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		if _, err := tx.Get("account", tenant, id); err == nil {
			return ErrConflict
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		rows, err := tx.List("account", tenant)
		if err != nil {
			return err
		}
		if len(rows) >= 100 {
			return ErrInvalid
		}
		if err = put(tx, "account", tenant, id, account, expires); err != nil {
			return err
		}
		return put(tx, "service_token", "global", account.TokenHash, account, expires)
	})
	if err != nil {
		return AccountView{}, "", err
	}
	return account.Public(), token, nil
}
func (s *Service) RevokeAccount(ctx context.Context, a Actor, tenant, id string) error {
	return s.repo.Update(ctx, "tenant:"+tenant, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		account, err := read[Account](tx, "account", tenant, id)
		if err != nil {
			return err
		}
		if err = tx.Delete("account", tenant, id); err != nil {
			return err
		}
		return tx.Delete("service_token", "global", account.TokenHash)
	})
}
func (s *Service) Directory(ctx context.Context, a Actor, tenant string) (any, error) {
	var result any
	err := s.repo.View(ctx, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		members, err := list[Member](tx, "member", tenant)
		if err != nil {
			return err
		}
		sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
		views := []MemberView{}
		for _, m := range members {
			u, e := read[User](tx, "user", "global", m.UserID)
			if e != nil {
				return e
			}
			u.Epoch = 0
			mapped, _, e := s.mappedGrants(tx, u)
			if e != nil {
				return e
			}
			if rank(mapped[tenant]) > rank(m.EffectiveRole) {
				m.EffectiveRole = mapped[tenant]
			}
			views = append(views, MemberView{m, u})
		}
		teams, err := list[Team](tx, "team", tenant)
		if err != nil {
			return err
		}
		sort.Slice(teams, func(i, j int) bool { return teams[i].ID < teams[j].ID })
		accounts, err := list[Account](tx, "account", tenant)
		if err != nil {
			return err
		}
		public := []AccountView{}
		for _, account := range accounts {
			public = append(public, account.Public())
		}
		sort.Slice(public, func(i, j int) bool { return public[i].ID < public[j].ID })
		federated, more, err := s.federatedDirectory(tx, tenant)
		if err != nil {
			return err
		}
		result = map[string]any{"members": views, "teams": teams, "serviceAccounts": public, "providers": s.Providers(), "ssoMappings": s.mappingViews(tenant), "ssoMembers": federated, "ssoMembersTruncated": more}
		return nil
	})
	return result, err
}
func (s *Service) RecordAudit(ctx context.Context, tenant string, a Actor, action, resource, outcome, requestID, revision string) error {
	now := s.Now().UTC()
	id := fmt.Sprintf("%020d-%s", now.UnixNano(), randomToken()[:12])
	event := Audit{id, now, a.ID, a.Kind, a.Name, action, resource, outcome, requestID, revision}
	return s.repo.Update(ctx, "audit:"+tenant, func(tx Tx) error { return put(tx, "audit", tenant, id, event, now.Add(90*24*time.Hour)) })
}
func (s *Service) AuditPage(ctx context.Context, a Actor, tenant, before string) ([]Audit, error) {
	out := []Audit{}
	err := s.repo.View(ctx, func(tx Tx) error {
		if err := s.checkOwner(tx, tenant, a); err != nil {
			return err
		}
		rows, err := tx.Page("audit", tenant, before, 200)
		if err != nil {
			return err
		}
		for _, b := range rows {
			var event Audit
			if err = json.Unmarshal(b, &event); err != nil {
				return err
			}
			out = append(out, event)
		}
		return nil
	})
	return out, err
}
func (s *Service) allowLogin(key string) bool {
	s.limitMu.Lock()
	defer s.limitMu.Unlock()
	now := s.Now()
	if len(s.limits) >= 10000 {
		for k, v := range s.limits {
			if now.Sub(v.at) > time.Hour {
				delete(s.limits, k)
			}
		}
		if len(s.limits) >= 10000 {
			return false
		}
	}
	v, ok := s.limits[key]
	if !ok {
		v.limiter = rate.NewLimiter(rate.Every(6*time.Second), 10)
	}
	v.at = now
	s.limits[key] = v
	return v.limiter.Allow()
}

func (s *Service) Ping(ctx context.Context) error {
	return s.repo.View(ctx, func(Tx) error { return nil })
}
func (s *Service) EnsureAdministration(ctx context.Context, legacy map[string]bool) error {
	return s.repo.View(ctx, func(tx Tx) error {
		for tenant := range s.tenants {
			if legacy[tenant] {
				continue
			}
			members, err := list[Member](tx, "member", tenant)
			if err != nil {
				return err
			}
			owner := false
			for _, m := range members {
				if m.Role == "owner" {
					owner = true
				}
			}
			if !owner {
				return fmt.Errorf("tenant %s has no owner: configure an exact-subject bootstrap owner or a legacy owner grant", tenant)
			}
		}
		return nil
	})
}
