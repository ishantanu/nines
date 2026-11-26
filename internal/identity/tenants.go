package identity

import (
	"context"
	"sort"
	"time"
)

// Tenants created by platform administrators while the server runs, as
// opposed to tenants defined in TENANTS_FILE. Every server and collector
// process reconciles the tenants it serves against this registry.
const (
	registryKind  = "tenant"
	registryScope = "global"

	TenantActive  = "active"
	TenantRetired = "retired"
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
