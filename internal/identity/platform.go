package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/systemone"
)

type PlatformAdmin struct {
	Provider string `yaml:"provider" json:"provider"`
	Subject  string `yaml:"subject" json:"subject"`
}
type PlatformTenant struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	SourceKind  string             `json:"sourceKind"`
	Connections int                `json:"connections"`
	SystemOne   *v1.SystemOneRoute `json:"systemOne,omitempty"`
}
type PlatformUser struct {
	User            User                 `json:"user"`
	PlatformAdmin   bool                 `json:"platformAdmin"`
	Local           bool                 `json:"local"`
	Memberships     []PlatformMembership `json:"memberships"`
	EffectiveGrants map[string]string    `json:"effectiveGrants"`
}
type PlatformMembership struct {
	Tenant string `json:"tenant"`
	Member Member `json:"member"`
}
type PlatformAudit struct {
	ID            string    `json:"id"`
	Cursor        string    `json:"cursor"`
	At            time.Time `json:"at"`
	ActorID       string    `json:"actorId"`
	ActorName     string    `json:"actorName,omitempty"`
	ActorProvider string    `json:"actorProvider,omitempty"`
	Action        string    `json:"action"`
	Resource      string    `json:"resource"`
	Reason        string    `json:"reason"`
	Outcome       string    `json:"outcome"`
	Details       any       `json:"details,omitempty"`
}
type PlatformAIPolicy struct {
	Version           int64 `json:"version"`
	Enabled           bool  `json:"enabled"`
	DailyRequestLimit int   `json:"dailyRequestLimit"`
}
type aiUsage struct {
	Date     string `json:"date"`
	Reserved int    `json:"reserved"`
}

func validatePlatformAdmins(c Config) error {
	if len(c.PlatformAdmins) > 20 {
		return fmt.Errorf("configure at most 20 platform administrators")
	}
	seen := map[string]bool{}
	providers := map[string]bool{}
	for _, p := range c.Providers {
		providers[p.ID] = true
	}
	for _, a := range c.PlatformAdmins {
		key := a.Provider + "\x00" + a.Subject
		if !providers[a.Provider] || strings.TrimSpace(a.Subject) == "" || len(a.Subject) > 500 || strings.ContainsRune(a.Subject, 0) || seen[key] {
			return fmt.Errorf("platformAdmins require unique known providers and exact nonempty subjects")
		}
		seen[key] = true
	}
	return nil
}
func (s *Service) isPlatformIdentity(u User) bool {
	if s.sessionProviderBinding(u) == "" {
		return false
	}
	for _, a := range s.config.PlatformAdmins {
		if a.Provider == u.Provider && a.Subject == u.Subject {
			return true
		}
	}
	return false
}
func (s *Service) platformAdminTx(tx Tx, a Actor) error {
	if !a.Session || a.Kind != "user" {
		return ErrForbidden
	}
	u, err := read[User](tx, "user", "global", a.ID)
	if err != nil {
		return err
	}
	session, sessionErr := read[Session](tx, "session", "global", a.sessionHash)
	if sessionErr != nil || session.UserID != u.ID || session.Epoch != u.Epoch || !session.Expires.After(s.Now()) || !equal(session.ProviderBinding, s.sessionProviderBinding(u)) {
		return ErrForbidden
	}
	if !s.isPlatformIdentity(u) {
		return ErrForbidden
	}
	return nil
}
func (s *Service) ConfigurePlatform(tenants []PlatformTenant, guards ...func([]byte) bool) {
	s.platformGuards = guards
	s.platformTenants = map[string]PlatformTenant{}
	for _, t := range tenants {
		s.platformTenants[t.ID] = t
	}
}
func (s *Service) recordPlatform(tx Tx, a Actor, action, resource, outcome string, details any) error {
	if !a.platformOperation {
		return nil
	}
	now := s.Now().UTC()
	id := model.NewID()
	cursor := fmt.Sprintf("%020d-%s", now.UnixNano(), id)
	event := PlatformAudit{ID: id, Cursor: cursor, At: now, ActorID: a.ID, ActorName: a.Name, ActorProvider: a.Provider, Action: action, Resource: resource, Reason: a.platformReason, Outcome: outcome, Details: details}
	return put(tx, "platform_audit", "global", cursor, event, now.Add(90*24*time.Hour))
}
func (s *Service) platformUsers(ctx context.Context, a Actor, before string) (any, error) {
	var out any
	err := s.repo.View(ctx, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		rows, err := tx.Page("user", "global", before, 51)
		if err != nil {
			return err
		}
		next := ""
		if len(rows) > 50 {
			rows = rows[:50]
			var last User
			if json.Unmarshal(rows[49], &last) != nil {
				return ErrInvalid
			}
			next = last.ID
		}
		users := []PlatformUser{}
		for _, raw := range rows {
			var u User
			if err = json.Unmarshal(raw, &u); err != nil {
				return err
			}
			grants, err := s.effectiveGrants(tx, u)
			if err != nil {
				return err
			}
			p := PlatformUser{User: u, PlatformAdmin: s.isPlatformIdentity(u), Local: s.providers[u.Provider].config.Type == "local", Memberships: []PlatformMembership{}, EffectiveGrants: grants}
			direct, err := list[grant](tx, "grant", u.ID)
			if err != nil {
				return err
			}
			for _, g := range direct {
				if s.tenants[g.Tenant] {
					p.Memberships = append(p.Memberships, PlatformMembership(g))
				}
			}
			sort.Slice(p.Memberships, func(i, j int) bool { return p.Memberships[i].Tenant < p.Memberships[j].Tenant })
			users = append(users, p)
		}
		out = map[string]any{"items": users, "nextCursor": next}
		return nil
	})
	return out, err
}
func (s *Service) revokePlatformSessions(ctx context.Context, a Actor, id string, expected int64) error {
	return s.repo.Update(ctx, "user:"+id, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		u, err := read[User](tx, "user", "global", id)
		if err != nil {
			return err
		}
		if expected < 0 || u.Version != expected {
			return ErrConflict
		}
		u.Epoch++
		u.Version++
		if err = put(tx, "user", "global", id, u, time.Time{}); err != nil {
			return err
		}
		return s.recordPlatform(tx, a, "sessions.revoke", id, "succeeded", map[string]any{"userVersion": u.Version})
	})
}
func policy(tx Tx, tenant string) (PlatformAIPolicy, error) {
	p, err := read[PlatformAIPolicy](tx, "platform_ai", "global", tenant)
	if errors.Is(err, ErrNotFound) {
		return PlatformAIPolicy{Enabled: true, DailyRequestLimit: 1000}, nil
	}
	return p, err
}
func (s *Service) platformAI(ctx context.Context, a Actor, tenant string) (any, error) {
	var out any
	err := s.repo.View(ctx, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		t, ok := s.platformTenants[tenant]
		if !ok {
			return ErrNotFound
		}
		p, err := policy(tx, tenant)
		if err != nil {
			return err
		}
		date := s.Now().UTC().Format("2006-01-02")
		u, err := read[aiUsage](tx, "platform_ai_usage", tenant, date)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		out = map[string]any{"tenant": tenant, "configured": t.SystemOne != nil, "route": t.SystemOne, "policy": p, "date": date, "reservedRequests": u.Reserved, "credentialStorage": "server-owned environment reference", "generativeModelsManaged": false}
		return nil
	})
	return out, err
}
func (s *Service) savePlatformAI(ctx context.Context, a Actor, tenant string, in PlatformAIPolicy) error {
	if in.Version < 0 || in.DailyRequestLimit < 1 || in.DailyRequestLimit > 100000 {
		return ErrInvalid
	}
	return s.repo.Update(ctx, "platform-ai:"+tenant, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		t, ok := s.platformTenants[tenant]
		if !ok {
			return ErrNotFound
		}
		if in.Enabled && t.SystemOne == nil {
			return fmt.Errorf("configure a server-owned System One route before enabling it: %w", ErrInvalid)
		}
		prior, err := policy(tx, tenant)
		if err != nil {
			return err
		}
		if in.Version != prior.Version {
			return ErrConflict
		}
		in.Version++
		if err = put(tx, "platform_ai", "global", tenant, in, time.Time{}); err != nil {
			return err
		}
		return s.recordPlatform(tx, a, "systemone.policy", tenant, "succeeded", map[string]any{"before": prior, "after": in})
	})
}

type controlledSystemOne struct {
	service *Service
	tenant  string
	inner   systemone.Assessor
}

func (s *Service) ControlSystemOne(tenant string, inner systemone.Assessor) systemone.Assessor {
	if existing, ok := inner.(*controlledSystemOne); ok && existing.service == s {
		if existing.tenant == tenant {
			return inner
		}
		inner = existing.inner
	}
	return &controlledSystemOne{s, tenant, inner}
}
func (c *controlledSystemOne) Descriptor() v1.SystemOneRoute { return c.inner.Descriptor() }
func (c *controlledSystemOne) ContainsCredential(raw []byte) bool {
	if guard, ok := c.inner.(interface{ ContainsCredential([]byte) bool }); ok {
		return guard.ContainsCredential(raw)
	}
	return false
}
func (c *controlledSystemOne) Assess(ctx context.Context, in systemone.Input) (v1.SystemOneAssessment, error) {
	if err := c.reserve(ctx); err != nil {
		return v1.SystemOneAssessment{}, err
	}
	return c.inner.Assess(ctx, in)
}

func (c *controlledSystemOne) reserve(ctx context.Context) error {
	err := c.service.repo.Update(ctx, "platform-ai:"+c.tenant, func(tx Tx) error {
		p, err := policy(tx, c.tenant)
		if err != nil {
			return err
		}
		if !p.Enabled {
			return systemone.ErrPaused
		}
		now := c.service.Now().UTC()
		date := now.Format("2006-01-02")
		u, err := read[aiUsage](tx, "platform_ai_usage", c.tenant, date)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if u.Reserved >= p.DailyRequestLimit {
			return systemone.ErrBudgetExhausted
		}
		u.Date = date
		u.Reserved++
		return put(tx, "platform_ai_usage", c.tenant, date, u, now.Add(48*time.Hour))
	})
	if err != nil {
		if errors.Is(err, systemone.ErrPaused) || errors.Is(err, systemone.ErrBudgetExhausted) {
			return err
		}
		return systemone.ErrPolicyUnavailable
	}
	return nil
}
func (c *controlledSystemOne) VisualEnabled() bool {
	v, ok := c.inner.(systemone.VisualAssessor)
	return ok && v.VisualEnabled()
}
func (c *controlledSystemOne) VisualDescriptor() v1.SystemOneRoute {
	if v, ok := c.inner.(systemone.VisualAssessor); ok {
		return v.VisualDescriptor()
	}
	return v1.SystemOneRoute{}
}
func (c *controlledSystemOne) AssessVisual(ctx context.Context, in systemone.VisualInput) (v1.VisualAssessment, error) {
	v, ok := c.inner.(systemone.VisualAssessor)
	if !ok || !v.VisualEnabled() {
		return v1.VisualAssessment{}, systemone.ErrPaused
	}
	if e := c.reserve(ctx); e != nil {
		return v1.VisualAssessment{}, e
	}
	return v.AssessVisual(ctx, in)
}

func (s *Service) platformReasonSafe(reason string) bool {
	for _, p := range s.providers {
		if len(p.config.secret) >= 12 && strings.Contains(reason, p.config.secret) {
			return false
		}
	}
	raw, _ := json.Marshal(reason)
	for _, guard := range s.platformGuards {
		if guard(raw) {
			return false
		}
	}
	return true
}
