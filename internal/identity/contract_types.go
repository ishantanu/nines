package identity

import "time"

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
