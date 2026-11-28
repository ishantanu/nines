package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Platform administration: tenants across the server. These calls need a
// configured platform administrator signed in through the browser or a CLI
// token from `nines auth login`; API keys and service accounts are refused.

// PlatformTenant is a tenant as the platform console lists it.
type PlatformTenant struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SourceKind  string `json:"sourceKind"`
	Connections int    `json:"connections"`
	// ManagedBy is "configuration" for tenants from the server's tenants
	// file, which cannot be changed here, or "console".
	ManagedBy string     `json:"managedBy"`
	State     string     `json:"state,omitempty"`
	Version   int64      `json:"version,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	RetiredAt *time.Time `json:"retiredAt,omitempty"`
}

// PlatformTenants lists the server's tenants. Provisioning is "platform
// console" when tenants can be created and changed through these calls.
type PlatformTenants struct {
	Tenants      []PlatformTenant `json:"tenants"`
	Provisioning string           `json:"tenantProvisioning"`
}

// RegisteredTenant is a tenant created by a platform administrator.
type RegisteredTenant struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"createdAt"`
	RetiredAt *time.Time `json:"retiredAt,omitempty"`
	Version   int64      `json:"version"`
}

// SetupLink is a one-time link for a new local account's owner to choose a
// password. Send it to them; it is shown only once.
type SetupLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// CreatedTenant is a new tenant, with its owner's setup link when the owner
// is a new local account.
type CreatedTenant struct {
	RegisteredTenant
	SetupLink *SetupLink `json:"setupLink,omitempty"`
}

// NewTenant creates a tenant with its first owner: an existing user
// (OwnerProvider and OwnerSubject) or a new local account (OwnerUser).
type NewTenant struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	OwnerProvider string         `json:"ownerProvider,omitempty"`
	OwnerSubject  string         `json:"ownerSubject,omitempty"`
	OwnerUser     *NewLocalOwner `json:"ownerUser,omitempty"`
}

// NewLocalOwner is a local account created as a tenant's first owner. It
// has no password until the owner uses the returned setup link.
type NewLocalOwner struct {
	Provider string `json:"provider"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
}

// PlatformError is a refused platform request, with the server's reason.
type PlatformError struct {
	StatusCode int
	Message    string
}

func (e *PlatformError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Nines platform API returned HTTP %d", e.StatusCode)
	}
	return e.Message
}

// Conflict reports a version or state conflict, such as a tenant changed by
// someone else since it was read.
func (e *PlatformError) Conflict() bool { return e.StatusCode == http.StatusConflict }

func (c *Client) platform(ctx context.Context, method, action string, body, out any) error {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+"/api/platform/"+action, payload)
	if err != nil {
		return err
	}
	// Platform requests name their tenant in the body, never in the
	// tenant header, which the server refuses here.
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("nines platform request failed")
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20+1))
	if err != nil || len(data) > 4<<20 {
		return errors.New("platform response unreadable or exceeds bounds")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &problem)
		if len(problem.Error) > 500 {
			problem.Error = problem.Error[:500]
		}
		return &PlatformError{StatusCode: res.StatusCode, Message: problem.Error}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// ListPlatformTenants lists every tenant on the server.
func (c *Client) ListPlatformTenants(ctx context.Context) (PlatformTenants, error) {
	var out PlatformTenants
	err := c.platform(ctx, http.MethodGet, "overview", nil, &out)
	return out, err
}

// CreateTenant creates a tenant and its first owner. It is served at once.
func (c *Client) CreateTenant(ctx context.Context, in NewTenant, reason string) (CreatedTenant, error) {
	var out CreatedTenant
	err := c.platform(ctx, http.MethodPost, "tenants/create", map[string]any{"reason": reason, "newTenant": in}, &out)
	return out, err
}

// RenameTenant changes a console tenant's display name at its expected
// version.
func (c *Client) RenameTenant(ctx context.Context, id, name string, expected int64, reason string) (RegisteredTenant, error) {
	var out RegisteredTenant
	err := c.platform(ctx, http.MethodPost, "tenants/rename", map[string]any{"reason": reason, "tenant": id, "name": name, "expectedVersion": expected}, &out)
	return out, err
}

// RetireTenant stops a console tenant's collection and alerting and makes
// it read-only, keeping its data.
func (c *Client) RetireTenant(ctx context.Context, id string, expected int64, reason string) (RegisteredTenant, error) {
	var out RegisteredTenant
	err := c.platform(ctx, http.MethodPost, "tenants/retire", map[string]any{"reason": reason, "tenant": id, "expectedVersion": expected}, &out)
	return out, err
}

// DeleteTenant permanently deletes a retired console tenant and its data.
// confirm must repeat the ID.
func (c *Client) DeleteTenant(ctx context.Context, id, confirm string, expected int64, reason string) error {
	return c.platform(ctx, http.MethodPost, "tenants/delete", map[string]any{"reason": reason, "tenant": id, "confirm": confirm, "expectedVersion": expected}, nil)
}

// IssueSetupLink gives a local user a new one-time setup link, replacing any
// earlier one. Using it sets a new password and signs them out everywhere.
func (c *Client) IssueSetupLink(ctx context.Context, userID, reason string) (SetupLink, error) {
	var out SetupLink
	err := c.platform(ctx, http.MethodPost, "users/setup-link", map[string]any{"reason": reason, "userId": userID}, &out)
	return out, err
}
