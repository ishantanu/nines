package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/ishantanu/nines/internal/tenancy"
)

// Mappings are server-owned policy, scoped by provider and exact trusted issuer.
// They are evaluated on each request, never copied into direct memberships.
func validateMappings(p *ProviderConfig) error {
	if p.Type != "oidc" {
		if len(p.RoleMappings) != 0 || p.ClaimsMaxAgeMinutes != 0 {
			return fmt.Errorf("SSO role mappings require an OIDC provider")
		}
		return nil
	}
	if p.ClaimsMaxAgeMinutes == 0 {
		p.ClaimsMaxAgeMinutes = 15
	}
	if p.ClaimsMaxAgeMinutes < 1 || p.ClaimsMaxAgeMinutes > 60 {
		return fmt.Errorf("claimsMaxAgeMinutes must be 1–60")
	}
	if len(p.RoleMappings) > 100 {
		return fmt.Errorf("configure at most 100 role mappings per provider")
	}
	ids, matches := map[string]bool{}, map[string]bool{}
	for _, m := range p.RoleMappings {
		key := m.Tenant + "\x00" + m.Claim + "\x00" + m.Value
		if !tenancy.ValidID(m.ID) || ids[m.ID] || !tenancy.ValidID(m.Tenant) || m.Claim != "groups" && m.Claim != "roles" || strings.TrimSpace(m.Value) != m.Value || m.Value == "" || len(m.Value) > 256 || strings.ContainsAny(m.Value, "\x00\r\n*") || !validRole(m.Role, true) || matches[key] {
			return fmt.Errorf("role mappings require unique IDs and matches, explicit tenant, groups/roles claim, exact value and valid role")
		}
		ids[m.ID], matches[key] = true, true
	}
	return nil
}

type authorizationClaims struct {
	Groups       []string  `json:"groups"`
	Roles        []string  `json:"roles"`
	GroupOverage bool      `json:"groupOverage"`
	IssuedAt     time.Time `json:"issuedAt"`
	FlowStarted  time.Time `json:"flowStarted"`
	TokenExpires time.Time `json:"tokenExpires"`
}

// Only called after ID-token signature, issuer, audience and nonce verification.
// Distributed-claim endpoints are deliberately never fetched.
func parseAuthorization(token *oidc.IDToken, now time.Time) (*authorizationClaims, error) {
	var claims map[string]json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return nil, ErrForbidden
	}
	out := &authorizationClaims{IssuedAt: token.IssuedAt, TokenExpires: token.Expiry}
	for _, field := range []struct {
		name   string
		target *[]string
		max    int
	}{{"groups", &out.Groups, 200}, {"roles", &out.Roles, 100}} {
		if b, ok := claims[field.name]; ok {
			if len(b) > 64<<10 || string(b) == "null" || json.Unmarshal(b, field.target) != nil || len(*field.target) > field.max {
				return nil, ErrForbidden
			}
			for _, v := range *field.target {
				if v == "" || len(v) > 256 || strings.ContainsAny(v, "\x00\r\n") {
					return nil, ErrForbidden
				}
			}
			slices.Sort(*field.target)
			*field.target = slices.Compact(*field.target)
		}
	}
	// Presence is enough to suppress group grants, even if a token also includes
	// a partial groups array or a false/malformed hasgroups value.
	if _, ok := claims["hasgroups"]; ok {
		out.GroupOverage = true
	}
	if b, ok := claims["_claim_names"]; ok {
		var names map[string]json.RawMessage
		if string(b) == "null" || json.Unmarshal(b, &names) != nil {
			return nil, ErrForbidden
		}
		if _, ok = names["groups"]; ok {
			out.GroupOverage = true
		}
	}
	if out.GroupOverage {
		out.Groups = nil
	}
	if out.IssuedAt.IsZero() || out.IssuedAt.After(now.Add(2*time.Minute)) {
		return nil, ErrForbidden
	}
	return out, nil
}

type federation struct {
	Provider string              `json:"provider"`
	Issuer   string              `json:"issuer"`
	ClientID string              `json:"clientId"`
	Claims   authorizationClaims `json:"claims"`
	Expires  time.Time           `json:"expiresAt"`
	Tenants  []string            `json:"tenants"`
}
type mappingView struct {
	RoleMapping
	Provider            string `json:"provider"`
	ClaimsMaxAgeMinutes int    `json:"claimsMaxAgeMinutes"`
}
type federatedMember struct {
	User          User      `json:"user"`
	MappedRole    string    `json:"mappedRole"`
	EffectiveRole string    `json:"effectiveRole"`
	MappingIDs    []string  `json:"mappingIds"`
	Expires       time.Time `json:"expiresAt"`
}

func (s *Service) mappingViews(tenant string) []mappingView {
	out := []mappingView{}
	for _, p := range s.providers {
		for _, m := range p.config.RoleMappings {
			if m.Tenant == tenant {
				out = append(out, mappingView{m, p.config.ID, p.config.ClaimsMaxAgeMinutes})
			}
		}
	}
	slices.SortFunc(out, func(a, b mappingView) int { return strings.Compare(a.Provider+"/"+a.ID, b.Provider+"/"+b.ID) })
	return out
}
func mappingExpiry(p ProviderConfig, f federation) time.Time {
	expires := f.Claims.IssuedAt.Add(time.Duration(p.ClaimsMaxAgeMinutes) * time.Minute)
	if f.Expires.Before(expires) {
		expires = f.Expires
	}
	if f.Claims.TokenExpires.Before(expires) {
		expires = f.Claims.TokenExpires
	}
	return expires
}
func (s *Service) evaluateMappings(p ProviderConfig, f federation) (map[string]string, map[string][]string) {
	grants, ids := map[string]string{}, map[string][]string{}
	if !mappingExpiry(p, f).After(s.Now()) || f.Provider != p.ID || f.Issuer != p.IssuerURL || f.ClientID != p.ClientID {
		return grants, ids
	}
	for _, m := range p.RoleMappings {
		if !s.tenants[m.Tenant] {
			continue
		}
		values := f.Claims.Roles
		if m.Claim == "groups" {
			if f.Claims.GroupOverage {
				continue
			}
			values = f.Claims.Groups
		}
		if !slices.Contains(values, m.Value) {
			continue
		}
		if rank(m.Role) > rank(grants[m.Tenant]) {
			grants[m.Tenant] = m.Role
		}
		ids[m.Tenant] = append(ids[m.Tenant], p.ID+"/"+m.ID)
	}
	return grants, ids
}
func (s *Service) mappedGrants(tx Tx, u User) (map[string]string, map[string][]string, error) {
	empty := map[string]string{}
	p, ok := s.providers[u.Provider]
	if !ok || p.config.Type != "oidc" || len(p.config.RoleMappings) == 0 {
		return empty, nil, nil
	}
	f, err := read[federation](tx, "federation", "global", u.ID)
	if errors.Is(err, ErrNotFound) {
		return empty, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	grants, ids := s.evaluateMappings(p.config, f)
	return grants, ids, nil
}
func (s *Service) effectiveGrants(tx Tx, u User) (map[string]string, error) {
	grants, _, err := s.mappedGrants(tx, u)
	if err != nil {
		return nil, err
	}
	rows, err := list[grant](tx, "grant", u.ID)
	if err != nil {
		return nil, err
	}
	for _, g := range rows {
		if !validRole(g.Member.EffectiveRole, true) {
			return nil, ErrForbidden
		}
		if s.tenants[g.Tenant] && rank(g.Member.EffectiveRole) > rank(grants[g.Tenant]) {
			grants[g.Tenant] = g.Member.EffectiveRole
		}
	}
	return grants, nil
}
func (s *Service) saveAuthorization(tx Tx, u User, claims *authorizationClaims, sessionExpires *time.Time) error {
	p := s.providers[u.Provider].config
	if p.Type != "oidc" {
		return nil
	}
	old, err := read[federation](tx, "federation", "global", u.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	// A delayed callback carrying older issuer evidence must not overwrite a
	// newer login's removals. Issuer timestamps may have only second precision;
	// the server-created flow start breaks ties between concurrent browser flows.
	if claims != nil && (old.Claims.IssuedAt.After(claims.IssuedAt) || old.Claims.IssuedAt.Equal(claims.IssuedAt) && old.Claims.FlowStarted.After(claims.FlowStarted)) {
		return ErrForbidden
	}
	previous, _ := s.evaluateMappings(p, old)
	for _, tenant := range old.Tenants {
		if err = tx.Delete("federated_member", tenant, u.ID); err != nil {
			return err
		}
	}
	if claims == nil { // No synthetic/legacy login may reuse another login's claims.
		return tx.Delete("federation", "global", u.ID)
	}
	expires := claims.IssuedAt.Add(time.Duration(p.ClaimsMaxAgeMinutes) * time.Minute)
	if claims.TokenExpires.Before(expires) {
		expires = claims.TokenExpires
	}
	if len(p.RoleMappings) > 0 {
		if !expires.After(s.Now()) {
			return ErrForbidden
		}
		if expires.Before(*sessionExpires) {
			*sessionExpires = expires
		}
	}
	f := federation{Provider: p.ID, Issuer: p.IssuerURL, ClientID: p.ClientID, Claims: *claims, Expires: expires, Tenants: []string{}}
	grants, _ := s.evaluateMappings(p, f)
	for tenant := range grants {
		f.Tenants = append(f.Tenants, tenant)
	}
	slices.Sort(f.Tenants)
	if err = put(tx, "federation", "global", u.ID, f, expires); err != nil {
		return err
	}
	for _, tenant := range f.Tenants {
		if err = put(tx, "federated_member", tenant, u.ID, map[string]string{"userId": u.ID}, expires); err != nil {
			return err
		}
	}
	// Login and derived-role changes are atomic. No raw group/role values enter
	// the audit log; policy mapping IDs are available in the owner directory.
	changed := map[string]bool{}
	for tenant := range previous {
		changed[tenant] = true
	}
	for tenant := range grants {
		changed[tenant] = true
	}
	for tenant := range changed {
		if previous[tenant] == grants[tenant] {
			continue
		}
		now := s.Now().UTC()
		id := fmt.Sprintf("%020d-%s", now.UnixNano(), randomToken()[:12])
		event := Audit{ID: id, At: now, ActorID: u.ID, ActorKind: "user", ActorName: u.Name, Action: "sso.role.changed", Resource: "user:" + u.ID, Outcome: fmt.Sprintf("%s -> %s", roleLabel(previous[tenant]), roleLabel(grants[tenant]))}
		if err = put(tx, "audit", tenant, id, event, now.Add(90*24*time.Hour)); err != nil {
			return err
		}
	}
	return nil
}
func roleLabel(role string) string {
	if role == "" {
		return "none"
	}
	return role
}
func (s *Service) federatedDirectory(tx Tx, tenant string) ([]federatedMember, bool, error) {
	rows, err := tx.Page("federated_member", tenant, "", 201)
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > 200
	if more {
		rows = rows[:200]
	}
	out := []federatedMember{}
	for _, b := range rows {
		var index struct {
			UserID string `json:"userId"`
		}
		if err = json.Unmarshal(b, &index); err != nil {
			return nil, false, err
		}
		u, e := read[User](tx, "user", "global", index.UserID)
		if e != nil {
			return nil, false, e
		}
		mapped, ids, e := s.mappedGrants(tx, u)
		if e != nil {
			return nil, false, e
		}
		if mapped[tenant] == "" {
			continue
		}
		effective, e := s.effectiveGrants(tx, u)
		if e != nil {
			return nil, false, e
		}
		f, e := read[federation](tx, "federation", "global", u.ID)
		if e != nil {
			return nil, false, e
		}
		u.Epoch = 0
		out = append(out, federatedMember{u, mapped[tenant], effective[tenant], ids[tenant], mappingExpiry(s.providers[u.Provider].config, f)})
	}
	slices.SortFunc(out, func(a, b federatedMember) int { return strings.Compare(a.User.ID, b.User.ID) })
	return out, more, nil
}
