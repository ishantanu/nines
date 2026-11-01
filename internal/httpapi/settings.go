package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/ishantanu/nines/internal/datasources"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/providers"
	"github.com/ishantanu/nines/internal/tenancy"
)

// These pages expose the actual server-owned configuration; they never return
// credentials or imply that read-only configuration can be edited through HTTP.
func (a *Server) access(w http.ResponseWriter, r *http.Request) {
	principal, _ := tenancy.FromContext(r.Context())
	type permission struct {
		ID    string          `json:"id"`
		Label string          `json:"label"`
		Roles map[string]bool `json:"roles"`
	}
	permissions := []permission{}
	for _, action := range []struct{ id, label, method, path string }{
		{"intentConversationsRead", "View intent conversations", "GET", "/api/intent-conversations"},
		{"intentConversationsWrite", "Prepare SLO intents", "POST", "/api/intent-conversations"},
		{"intentBindingConfirm", "Confirm intent measurement semantics", "POST", "/api/intent-conversations/example/selection"},
		{"intentChangeReview", "Approve and apply reviewed intent definition changes", "POST", "/api/intent-conversations/example/change-set/apply"},
		{"replayRead", "View tenant session replays", "GET", "/api/session-replays"},
		{"replayWrite", "Ingest and link masked session replays", "POST", "/api/session-replays"},
		{"replayArchive", "Archive or restore session replays", "POST", "/api/session-replays/example/archive"},
		{"read", "Read tenant resources", "GET", "/api/slos"},
		{"plan", "Validate and plan definitions", "POST", "/api/resources/plan"},
		{"propose", "Start investigations and proposals", "POST", "/api/workflows"},
		{"evidence", "Record run evidence", "POST", "/api/journeys/example/runs"},
		{"apply", "Apply SLO, journey, binding and dependency-map definitions", "POST", "/api/resources/apply"},
		{"review", "Approve or reject proposals", "POST", "/api/workflows/example/approve"},
		{"testConnection", "Test a configured data source", "POST", "/api/connections/primary/test"},
		{"planRules", "Preview recording-rule destination", "POST", "/api/slos/example/rules/plan"},
		{"deployRules", "Deploy recording and alerting rules", "POST", "/api/slos/example/rules/apply"},
		{"ruleAutomation", "Pause or resume automatic rule deployment", "POST", "/api/slos/example/rules/automation"},
	} {
		grants := map[string]bool{}
		for _, role := range []string{"owner", "editor", "agent", "viewer"} {
			request, _ := http.NewRequest(action.method, action.path, nil)
			grants[role] = allowed(role, request)
		}
		permissions = append(permissions, permission{action.id, action.label, grants})
	}
	bindings := []map[string]string{}
	if principal.Role == "owner" {
		for i, g := range a.Workspace.Grants {
			ref := "Server API token"
			if i < len(a.Workspace.AccessConfig) {
				ref = a.Workspace.AccessConfig[i].TokenEnv
			}
			bindings = append(bindings, map[string]string{"credentialRef": ref, "role": g.Role})
		}
	}
	authentication := map[bool]string{true: "anonymous_local", false: "api_token"}[a.Workspace.Anonymous]
	switch principal.Kind {
	case "user":
		authentication = "browser_session"
	case "service_account":
		authentication = "service_account"
	}
	write(w, 200, map[string]any{"tenant": principal.TenantID, "currentRole": principal.Role, "management": "server_configuration", "authentication": authentication, "permissions": permissions, "bindings": bindings, "bindingsVisible": principal.Role == "owner", "userManagementSupported": a.Workspace.Identity != nil})
}
func (a *Server) connections(w http.ResponseWriter, r *http.Request) {

	items := []map[string]any{}
	connections := a.Workspace.Connections
	if len(connections) == 0 && a.Service.Collector != nil {
		connections = []tenancy.Connection{{Name: "primary", Source: a.Workspace.Source}}
	}
	for _, connection := range connections {
		if connection.Kind == "manual" {
			continue
		}
		auth := "none"
		if len(connection.CredentialsEnv) > 0 {
			auth = "provider_credentials"
		}
		if connection.BearerTokenEnv != "" {
			auth = "bearer"
		} else if connection.BasicUsername != "" {
			auth = "basic"
		} else if connection.Name == "primary" && a.Workspace.SourceAuth != "" {
			auth = a.Workspace.SourceAuth
		}
		descriptor, _ := providers.Default.Describe(connection.Kind)
		items = append(items, map[string]any{"capabilities": descriptor.Capabilities, "queryLanguages": descriptor.QueryLanguages, "id": connection.Name, "kind": connection.Kind, "endpoint": connection.URL, "mimirOrgID": connection.MimirOrgID, "authentication": auth, "configuration": "server_configuration", "collectionMode": connection.CollectionMode, "rules": connection.Rules, "options": connection.Options})
	}
	managed := a.Workspace.ManagedSources != nil && a.Workspace.ManagedSources.Enabled()
	if a.Workspace.ManagedSources != nil {
		items = append(items, a.Workspace.ManagedSources.List()...)
	}
	routes := datasources.ProviderRoutes{}
	if a.Workspace.ManagedSources != nil {
		routes = a.Workspace.ManagedSources.AllowedProviderRoutes()
	}
	write(w, 200, map[string]any{"management": "server_configuration", "managedConnectionsEnabled": managed, "providers": providers.Default.List(), "connections": items, "manualIngestion": true, "maxConnections": 21, "managedProviderRoutes": routes})
}

func (a *Server) connectionTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("connection")
	if id == "" {
		id = "primary"
	}
	collector := a.Service.CollectorFor(model.SLO{Source: id})
	if collector == nil {
		a.fail(w, r, model.ErrUnavailable)
		return
	}
	// Only the server-configured destination and credentials are used; no URL,
	// tenant header or query expression can be supplied by the caller.
	if r.ContentLength != 0 {
		a.fail(w, r, model.Invalid("connection test takes no request body"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if probe, ok := collector.(interface{ Probe(context.Context) error }); ok {
		if err := probe.Probe(ctx); err != nil {
			write(w, 502, map[string]string{"status": "failed", "error": "Configured provider authentication probe failed; check credentials and availability"})
			return
		}
		write(w, 200, map[string]any{"status": "reachable", "testedAt": time.Now().UTC(), "scope": "configured tenant", "check": "Provider authentication probe only; metric permissions and SLO coverage are not verified"})
		return
	}
	at := time.Now().UTC().Truncate(time.Minute)
	points, err := collector.Collect(ctx, model.SLO{StepSeconds: 60, GoodQuery: "vector(1)", TotalQuery: "vector(1)"}, at.Add(-time.Minute), at)
	if err != nil || len(points) != 1 {
		write(w, 502, map[string]string{"status": "failed", "error": "Configured source query failed; check server configuration and source availability"})
		return
	}
	write(w, 200, map[string]any{"status": "reachable", "testedAt": time.Now().UTC(), "scope": "configured tenant", "check": "Prometheus range query; no samples stored"})
}
