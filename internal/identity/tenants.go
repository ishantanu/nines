package identity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/tenancy"
)

// Tenants created by platform administrators while the server runs, as
// opposed to tenants defined in TENANTS_FILE. Every server and collector
// process reconciles the tenants it serves against this registry.
const (
	registryKind  = "tenant"
	registryScope = "global"

	TenantActive  = "active"
	TenantRetired = "retired"
	// TenantDeleting is a retired tenant whose data is being deleted. It is
	// no longer served; deleting again resumes an interrupted deletion.
	TenantDeleting = "deleting"
)

// RegisteredTenant is a tenant created in the platform console.
type RegisteredTenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	// RetiredAt is set while the tenant is retired: read-only, not collected
	// and not alerting, with its data kept.
	RetiredAt *time.Time `json:"retiredAt,omitempty"`
	Version   int64      `json:"version"`
}

// RegisteredTenants lists the registry, by ID. It needs only the repository,
// so collector processes without identity configuration can read it.
func RegisteredTenants(ctx context.Context, repo Repository) ([]RegisteredTenant, error) {
	var out []RegisteredTenant
	err := repo.View(ctx, func(tx Tx) error {
		var err error
		out, err = list[RegisteredTenant](tx, registryKind, registryScope)
		return err
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, err
}

// TenantInput creates or changes a console tenant. The first owner is an
// existing user (Provider and Subject) or a new local account (User).
type TenantInput struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	OwnerProvider   string          `json:"ownerProvider,omitempty"`
	OwnerSubject    string          `json:"ownerSubject,omitempty"`
	OwnerUser       *LocalUserInput `json:"ownerUser,omitempty"`
	ExpectedVersion int64           `json:"expectedVersion"`
	// Confirm must repeat the ID to delete a tenant and its data.
	Confirm string `json:"confirm,omitempty"`
}

// ErrTenantConfigured refuses changes to a tenant defined in TENANTS_FILE.
var ErrTenantConfigured = fmt.Errorf("this tenant is managed by server configuration: %w", ErrConflict)

func validTenantName(name string) bool {
	return strings.TrimSpace(name) != "" && len(name) <= 200 && utf8.ValidString(name) && !strings.ContainsAny(name, "\x00\r\n")
}

// CreateTenant registers a tenant and its first owner in one transaction.
// Every process then serves it: this one at once, others within seconds.
func (s *Service) CreateTenant(ctx context.Context, a Actor, in TenantInput) (_ RegisteredTenant, err error) {
	ctx, finish := observability.Operation(ctx, "tenant.create", in.ID)
	defer func() { finish(err) }()
	t := RegisteredTenant{ID: in.ID, Name: strings.TrimSpace(in.Name), State: TenantActive, CreatedAt: s.Now().UTC(), CreatedBy: a.ID, Version: 1}
	// default maps to the base database schema and is never a console tenant.
	if !tenancy.ValidID(in.ID) || in.ID == "default" || !validTenantName(in.Name) || in.ExpectedVersion != 0 || in.Confirm != "" {
		return t, ErrInvalid
	}
	existing := in.OwnerProvider != "" || in.OwnerSubject != ""
	if existing == (in.OwnerUser != nil) {
		return t, fmt.Errorf("choose an existing user or a new local account as the first owner: %w", ErrInvalid)
	}
	var owner User
	var hash []byte
	if existing {
		if _, ok := s.providers[in.OwnerProvider]; !ok || len(in.OwnerSubject) > 500 {
			return t, ErrInvalid
		}
		owner = s.user(in.OwnerProvider, externalUser{Subject: in.OwnerSubject})
	} else {
		u := *in.OwnerUser
		p, ok := s.providers[u.Provider]
		if !ok || p.config.Type != "local" || !localUsername.MatchString(u.Username) || strings.TrimSpace(u.Name) == "" || len(u.Name) > 300 || len(u.Email) > 320 || (u.Role != "" && u.Role != "owner") || u.ExpectedVersion != 0 {
			return t, ErrInvalid
		}
		var err error
		if hash, err = passwordHash(u.Password); err != nil {
			return t, err
		}
		owner = s.user(u.Provider, externalUser{Subject: u.Username, Name: u.Name, Email: u.Email})
		owner.Version = 1
	}
	if s.knownTenant(in.ID) {
		return t, fmt.Errorf("a tenant with this ID exists: %w", ErrConflict)
	}
	err = s.repo.Update(ctx, "tenant:"+in.ID, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		if _, err := tx.Get(registryKind, registryScope, in.ID); err == nil {
			return fmt.Errorf("a tenant with this ID exists: %w", ErrConflict)
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := put(tx, registryKind, registryScope, t.ID, t, time.Time{}); err != nil {
			return err
		}
		if err := s.recordPlatform(tx, a, "tenant.create", t.ID, "succeeded", map[string]any{"name": t.Name, "owner": owner.Provider + "/" + owner.Subject}); err != nil {
			return err
		}
		if existing {
			_, err := s.grantTx(tx, a, t.ID, owner, "owner", 0)
			return err
		}
		return s.createLocalUserTx(tx, a, t.ID, owner, hash, "owner")
	})
	if err != nil {
		return t, err
	}
	s.AddTenant(t.ID)
	return t, nil
}

// updateTenant changes a console tenant at its expected version.
func (s *Service) updateTenant(ctx context.Context, a Actor, id string, expected int64, action string, change func(*RegisteredTenant) error) (RegisteredTenant, error) {
	var out RegisteredTenant
	if s.configured[id] {
		return out, ErrTenantConfigured
	}
	err := s.repo.Update(ctx, "tenant:"+id, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		t, err := read[RegisteredTenant](tx, registryKind, registryScope, id)
		if err != nil {
			return err
		}
		if t.Version != expected {
			return ErrConflict
		}
		before := t
		if err = change(&t); err != nil {
			return err
		}
		t.Version++
		out = t
		if err = put(tx, registryKind, registryScope, id, t, time.Time{}); err != nil {
			return err
		}
		return s.recordPlatform(tx, a, action, id, "succeeded", map[string]any{"from": before.Name, "to": t.Name, "state": t.State})
	})
	return out, err
}

// RenameTenant changes a console tenant's display name. The ID never changes.
func (s *Service) RenameTenant(ctx context.Context, a Actor, id, name string, expected int64) (_ RegisteredTenant, err error) {
	ctx, finish := observability.Operation(ctx, "tenant.rename", id)
	defer func() { finish(err) }()
	if !validTenantName(name) {
		return RegisteredTenant{}, ErrInvalid
	}
	return s.updateTenant(ctx, a, id, expected, "tenant.rename", func(t *RegisteredTenant) error {
		t.Name = strings.TrimSpace(name)
		return nil
	})
}

// RetireTenant stops a console tenant's collection and alerting and makes it
// read-only. Its data stays until it is deleted.
func (s *Service) RetireTenant(ctx context.Context, a Actor, id string, expected int64) (_ RegisteredTenant, err error) {
	ctx, finish := observability.Operation(ctx, "tenant.retire", id)
	defer func() { finish(err) }()
	return s.updateTenant(ctx, a, id, expected, "tenant.retire", func(t *RegisteredTenant) error {
		if t.State == TenantRetired {
			return fmt.Errorf("the tenant is already retired: %w", ErrConflict)
		}
		now := s.Now().UTC()
		t.State, t.RetiredAt = TenantRetired, &now
		return nil
	})
}

// DeleteTenant deletes a retired console tenant: deleteData removes its
// stored data, then its registration, members and teams go. Its credentials
// stop working at once. confirm must repeat the ID. If deleteData fails, the
// tenant stays deleting and deleting it again resumes.
func (s *Service) DeleteTenant(ctx context.Context, a Actor, id, confirm string, expected int64, deleteData func(context.Context) error) (err error) {
	ctx, finish := observability.Operation(ctx, "tenant.delete", id)
	defer func() { finish(err) }()
	if s.configured[id] {
		return ErrTenantConfigured
	}
	if confirm != id || id == "default" {
		return fmt.Errorf("type the tenant ID to confirm deletion: %w", ErrInvalid)
	}
	marked, err := s.updateTenant(ctx, a, id, expected, "tenant.delete.start", func(t *RegisteredTenant) error {
		if t.State != TenantRetired && t.State != TenantDeleting {
			return fmt.Errorf("retire the tenant before deleting it: %w", ErrConflict)
		}
		t.State = TenantDeleting
		return nil
	})
	if err != nil {
		return err
	}
	s.RemoveTenant(id)
	if err = deleteData(ctx); err != nil {
		return fmt.Errorf("the tenant's data could not be deleted; delete it again to retry: %w", err)
	}
	return s.repo.Update(ctx, "tenant:"+id, func(tx Tx) error {
		if err := s.platformAdminTx(tx, a); err != nil {
			return err
		}
		t, err := read[RegisteredTenant](tx, registryKind, registryScope, id)
		if err != nil {
			return err
		}
		if t.Version != marked.Version {
			return ErrConflict
		}
		members, err := list[Member](tx, "member", id)
		if err != nil {
			return err
		}
		for _, m := range members {
			if err = tx.Delete("member", id, m.UserID); err != nil {
				return err
			}
			if err = tx.Delete("grant", m.UserID, id); err != nil {
				return err
			}
		}
		teams, err := list[Team](tx, "team", id)
		if err != nil {
			return err
		}
		for _, team := range teams {
			if err = tx.Delete("team", id, team.ID); err != nil {
				return err
			}
		}
		if err = tx.Delete(registryKind, registryScope, id); err != nil {
			return err
		}
		return s.recordPlatform(tx, a, "tenant.delete", id, "succeeded", map[string]any{"name": t.Name, "members": len(members), "teams": len(teams)})
	})
}

// platformTenantList is every tenant the console shows: those served here,
// and console tenants from the registry, including ones being deleted.
func (s *Service) platformTenantList(ctx context.Context) ([]PlatformTenant, error) {
	byID := map[string]PlatformTenant{}
	for _, t := range s.platformInventory() {
		t.ManagedBy = "configuration"
		if !s.configured[t.ID] {
			t.ManagedBy = "console"
		}
		byID[t.ID] = t
	}
	if s.TenantsChanged != nil {
		registered, err := RegisteredTenants(ctx, s.repo)
		if err != nil {
			return nil, err
		}
		for _, r := range registered {
			if s.configured[r.ID] {
				continue
			}
			t, ok := byID[r.ID]
			if !ok {
				t = PlatformTenant{ID: r.ID, SourceKind: "manual"}
			}
			created := r.CreatedAt
			t.Name, t.ManagedBy, t.State, t.Version, t.CreatedAt, t.RetiredAt = r.Name, "console", r.State, r.Version, &created, r.RetiredAt
			byID[r.ID] = t
		}
	}
	out := make([]PlatformTenant, 0, len(byID))
	for _, t := range byID {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// platformTenantChange applies a console tenant change and serves it on
// this process before replying.
func (s *Service) platformTenantChange(ctx context.Context, a Actor, action string, in PlatformRequest) (any, error) {
	if s.TenantsChanged == nil {
		return nil, fmt.Errorf("tenants are managed by server configuration on this server: %w", ErrConflict)
	}
	var out any
	var err error
	switch action {
	case "tenants/create":
		if in.NewTenant == nil {
			return nil, ErrInvalid
		}
		out, err = s.CreateTenant(ctx, a, *in.NewTenant)
	case "tenants/rename":
		out, err = s.RenameTenant(ctx, a, in.Tenant, in.Name, in.ExpectedVersion)
	case "tenants/retire":
		out, err = s.RetireTenant(ctx, a, in.Tenant, in.ExpectedVersion)
	case "tenants/delete":
		if s.DeleteTenantData == nil {
			return nil, fmt.Errorf("this server cannot delete tenant data: %w", ErrConflict)
		}
		err = s.DeleteTenant(ctx, a, in.Tenant, in.Confirm, in.ExpectedVersion, func(ctx context.Context) error {
			// Stop serving it here before its data goes.
			if err := s.TenantsChanged(ctx); err != nil {
				return err
			}
			return s.DeleteTenantData(ctx, in.Tenant)
		})
		out = StatusResult{Status: "deleted"}
	}
	if err != nil {
		return nil, err
	}
	if err = s.TenantsChanged(ctx); err != nil {
		return nil, fmt.Errorf("saved, but this server could not apply it yet; it retries within seconds: %w", err)
	}
	return out, nil
}
