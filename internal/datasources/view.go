package datasources

import (
	"github.com/ishantanu/nines/internal/telemetryprovider"
	"github.com/ishantanu/nines/internal/tenancy"
)

// ConnectionView is a telemetry connection as the API shows it: never its
// credentials, only how it authenticates. Configuration says whether the
// server's configuration or the tenant (managed) owns it.
type ConnectionView struct {
	Capabilities   telemetryprovider.Capabilities `json:"capabilities"`
	QueryLanguages []string                       `json:"queryLanguages"`
	ID             string                         `json:"id"`
	Kind           string                         `json:"kind"`
	Endpoint       string                         `json:"endpoint"`
	MimirOrgID     string                         `json:"mimirOrgID"`
	Authentication string                         `json:"authentication"`
	Configuration  string                         `json:"configuration"`
	// Version is the revision of a managed connection.
	Version        int64                    `json:"version,omitempty"`
	Rules          *tenancy.RuleDestination `json:"rules"`
	Options        map[string]string        `json:"options"`
	CollectionMode string                   `json:"collectionMode"`
}
