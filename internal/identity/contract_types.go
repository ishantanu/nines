package identity

import (
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Request and response bodies of the team access API, named so the OpenAPI
// contract is generated from, and checked against, the shapes encoded.

// TeamDirectory is the body of GET /api/team-access.
type TeamDirectory struct {
	Members             []MemberView      `json:"members"`
	Teams               []Team            `json:"teams"`
	ServiceAccounts     []AccountView     `json:"serviceAccounts"`
	Providers           []ProviderView    `json:"providers"`
	SSOMappings         []mappingView     `json:"ssoMappings"`
	SSOMembers          []federatedMember `json:"ssoMembers"`
	SSOMembersTruncated bool              `json:"ssoMembersTruncated"`
}

// AuditList is a page of team access audit records.
type AuditList struct {
	Items      []Audit `json:"items"`
	NextCursor string  `json:"nextCursor"`
}

// GrantInput grants a provider user a role in the tenant.
type GrantInput struct {
	Provider string `json:"provider"`
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	Expected int64  `json:"expectedVersion"`
}

// TeamInput creates or updates a team at its expected version.
type TeamInput struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Members  []string `json:"members"`
	Expected int64    `json:"expectedVersion"`
}

// ServiceAccountInput creates a service account.
type ServiceAccountInput struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Role    string    `json:"role"`
	Expires time.Time `json:"expiresAt"`
}

// CreatedServiceAccount returns the account and its token, shown once.
type CreatedServiceAccount struct {
	Account AccountView `json:"account"`
	Token   string      `json:"token"`
}

// StatusResult reports a removal or revocation.
type StatusResult struct {
	Status string `json:"status"`
}

// CLIStartInput starts a browser sign-in for the CLI. The CLI listens on a
// loopback redirect URI and proves possession with a PKCE challenge.
type CLIStartInput struct {
	Tenant    string `json:"tenant"`
	Redirect  string `json:"redirectURI"`
	Challenge string `json:"challenge"`
	State     string `json:"state"`
}

// CLIStarted is where the CLI sends the browser, and until when.
type CLIStarted struct {
	AuthorizationURL string    `json:"authorizationURL"`
	Expires          time.Time `json:"expiresAt"`
}

// CLIExchangeInput trades the one-time code from the browser for a token.
type CLIExchangeInput struct {
	Code     string `json:"code"`
	Verifier string `json:"codeVerifier"`
	Redirect string `json:"redirectURI"`
}

// CLIToken is a CLI credential for one tenant, shown once.
type CLIToken struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expiresAt"`
	Tenant  string    `json:"tenant"`
	Actor   Actor     `json:"actor"`
	Role    string    `json:"role"`
}

// PlatformOverview is the body of GET /api/platform/overview.
type PlatformOverview struct {
	Tenants                     []PlatformTenant `json:"tenants"`
	Providers                   []ProviderView   `json:"providers"`
	Administrators              []PlatformAdmin  `json:"administrators"`
	AdministratorRolesManagedBy string           `json:"administratorRolesManagedBy"`
	// TenantProvisioning is "platform console" when tenants can be created
	// and changed through the platform API, else "server configuration".
	TenantProvisioning string `json:"tenantProvisioning"`
	AuditRetentionDays int    `json:"auditRetentionDays"`
}

// TenantCreateRequest is the body of POST /api/platform/tenants/create.
type TenantCreateRequest struct {
	Reason    string      `json:"reason"`
	NewTenant TenantInput `json:"newTenant"`
}

// TenantChangeRequest is the body of POST /api/platform/tenants/rename,
// retire and delete: the tenant, the version the caller read, and for
// rename the new name or for delete the ID repeated.
type TenantChangeRequest struct {
	Reason          string `json:"reason"`
	Tenant          string `json:"tenant"`
	Name            string `json:"name,omitempty"`
	Confirm         string `json:"confirm,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// PlatformUserPage is a page of users across tenants.
type PlatformUserPage struct {
	Items      []PlatformUser `json:"items"`
	NextCursor string         `json:"nextCursor"`
}

// PlatformAuditPage is a page of platform audit records, newest first.
type PlatformAuditPage struct {
	Items      []PlatformAudit `json:"items"`
	NextCursor string          `json:"nextCursor"`
}

// PlatformAIStatus is a tenant's Jev (System One) policy and today's usage.
type PlatformAIStatus struct {
	Tenant                  string             `json:"tenant"`
	Configured              bool               `json:"configured"`
	Route                   *v1.SystemOneRoute `json:"route"`
	Policy                  PlatformAIPolicy   `json:"policy"`
	Date                    string             `json:"date"`
	ReservedRequests        int                `json:"reservedRequests"`
	CredentialStorage       string             `json:"credentialStorage"`
	GenerativeModelsManaged bool               `json:"generativeModelsManaged"`
}

// PlatformOK acknowledges a platform change with no other result.
type PlatformOK struct {
	OK bool `json:"ok"`
}
