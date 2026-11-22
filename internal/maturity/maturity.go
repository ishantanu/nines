// Package maturity is the single source of truth for what each Nines
// capability promises. The CLI reports it, the portal shows it from a
// generated copy, and the documentation table is checked against it.
package maturity

// Level is a compatibility promise, not a quality grade.
type Level string

const (
	// Stable capabilities follow the compatibility and deprecation policy.
	Stable Level = "stable"
	// Beta capabilities are the stable core in waiting: complete, tested end
	// to end and frozen against breaking changes except through deprecation.
	Beta Level = "beta"
	// Experimental capabilities are opt-in and may change or be removed.
	Experimental Level = "experimental"
)

// Definition states what a level promises users.
type Definition struct {
	Level   Level  `json:"level"`
	Name    string `json:"name"`
	Promise string `json:"promise"`
}

// Feature is one capability users can see or depend on.
type Feature struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level Level  `json:"level"`
	// Docs is the guide relative to docs/public, without the .md extension.
	Docs string `json:"docs"`
	// Page is the portal page the feature lives on, when it has one.
	Page string `json:"page,omitempty"`
	// Provider is the telemetry provider kind, for provider adapters.
	Provider string `json:"provider,omitempty"`
	// Remaining states what stands between an experimental feature and beta.
	Remaining string `json:"remaining,omitempty"`
	// Gated experimental features are off until a server turns them on with
	// a feature gate. Ungated experimental features are already opt-in
	// through their own configuration.
	Gated bool `json:"gated,omitempty"`
}

// Levels returns the level definitions, most to least committed.
func Levels() []Definition {
	return []Definition{
		{Stable, "Stable", "Covered by the compatibility and deprecation policy. Definitions, API fields, CLI flags and behavior change only through announced deprecation."},
		{Beta, "Beta", "Feature complete and tested end to end. From the first beta release, breaking changes go through deprecation; these features become stable with the first stable release."},
		{Experimental, "Experimental", "Opt-in and may change or be removed in any release without deprecation. Use it to evaluate, not to depend on."},
	}
}

// Catalog lists every user-facing capability with its current level.
func Catalog() []Feature {
	return []Feature{
		// The stable core, in beta until the first stable release.
		{ID: "slo-definitions", Name: "SLO definitions and SLO types", Level: Beta, Docs: "slo-types", Page: "objectives"},
		{ID: "slo-lifecycle", Name: "SLO lifecycle, editing and retirement", Level: Beta, Docs: "slo-retirement"},
		{ID: "slo-templates", Name: "SLO templates", Level: Beta, Docs: "slo-templates"},
		{ID: "journeys", Name: "Critical user journeys and run objectives", Level: Beta, Docs: "critical-user-journeys", Page: "journeys"},
		{ID: "burn-alerts", Name: "Recording rules and burn-rate alerts", Level: Beta, Docs: "deployment/metrics-and-rules", Page: "alerts"},
		{ID: "error-budget-policy", Name: "Error budget policies", Level: Beta, Docs: "error-budget-policy"},
		{ID: "measurement-quality", Name: "Labels, per-customer reports and cohorts", Level: Beta, Docs: "measurement-quality"},
		{ID: "service-dependencies", Name: "Declared service dependency maps", Level: Beta, Docs: "service-dependencies", Page: "maps"},
		{ID: "prometheus", Name: "Prometheus", Level: Beta, Docs: "provider-adapters", Provider: "prometheus"},
		{ID: "mimir", Name: "Grafana Mimir", Level: Beta, Docs: "provider-adapters", Provider: "mimir"},
		{ID: "portal", Name: "Web portal", Level: Beta, Docs: "index"},
		{ID: "cli", Name: "CLI", Level: Beta, Docs: "../cli", Page: "clients"},
		{ID: "http-api", Name: "HTTP API and Go client", Level: Beta, Docs: "api"},
		{ID: "mcp", Name: "MCP server over stdio", Level: Beta, Docs: "mcp"},
		{ID: "identity", Name: "Local accounts, OIDC, LDAP and RBAC", Level: Beta, Docs: "deployment/identity-and-tenants", Page: "access"},
		{ID: "tenancy", Name: "Tenants and telemetry connections", Level: Beta, Docs: "configuration", Page: "connections"},
		{ID: "audit-log", Name: "Audit log", Level: Beta, Docs: "operations", Page: "audit"},
		{ID: "operator", Name: "Kubernetes operator", Level: Beta, Docs: "deployment/operator"},

		// Experimental.
		{ID: "create-from-goal", Name: "Create from a goal", Level: Experimental, Docs: "intents", Page: "intents", Remaining: "Freeze the intent schema and pass real-provider acceptance for model drafting (#34).", Gated: true},
		{ID: "reliability-reviews", Name: "Reliability reviews", Level: Experimental, Docs: "architecture", Page: "investigations", Remaining: "Settle the review schema and its link to incidents and postmortems.", Gated: true},
		{ID: "incidents", Name: "Incident records", Level: Experimental, Docs: "incidents", Page: "incidents", Remaining: "Portal pages, postmortems and links to incident tools (#86-#90), and acceptance with a real incident workflow.", Gated: true},
		{ID: "workflows", Name: "Workflows and webhooks", Level: Experimental, Docs: "workflow-webhooks", Page: "workflows", Remaining: "Freeze the workflow event contract and accept it with a real webhook consumer.", Gated: true},
		{ID: "agent-evidence", Name: "Agent evidence and visual evaluations", Level: Experimental, Docs: "visual-evaluations", Page: "agents", Remaining: "Freeze the run evidence schema and accept it with real agent workloads.", Gated: true},
		{ID: "session-replay", Name: "Session replay", Level: Experimental, Docs: "session-replay", Page: "replays", Remaining: "Browser and platform compatibility, a masking and privacy review, and offline-player acceptance.", Gated: true},
		{ID: "git-delivery", Name: "Git delivery", Level: Experimental, Docs: "git-delivery", Remaining: "Acceptance against live GitHub and GitLab, not only API fakes."},
		{ID: "migration", Name: "Migration from Sloth and Pyrra", Level: Experimental, Docs: "migration", Remaining: "Acceptance with real-world Sloth and Pyrra files beyond the projects' published examples.", Gated: true},
		{ID: "observed-dependencies", Name: "Observed dependency evidence", Level: Experimental, Docs: "service-dependencies", Remaining: "Instrumentation coverage and production-scale validation.", Gated: true},
		{ID: "remote-mcp", Name: "Remote MCP over HTTP with OAuth", Level: Experimental, Docs: "remote-mcp", Remaining: "Hosted agent-client interoperability and production issuer acceptance."},
		{ID: "audit-archive", Name: "Immutable audit archival", Level: Experimental, Docs: "deployment/audit-archive", Remaining: "Production IAM and KMS, high availability and real AWS acceptance."},
		{ID: "github-reviews", Name: "GitHub review comments", Level: Experimental, Docs: "github-reviews", Remaining: "Settle together with Git delivery so one GitHub integration remains."},
		{ID: "slo-extensions", Name: "SLO template extensions", Level: Experimental, Docs: "slo-extensions", Remaining: "A stable extension contract; Go extensions need a custom build today."},
		{ID: "openslo", Name: "OpenSLO export", Level: Experimental, Docs: "openslo", Remaining: "Round-trip acceptance with OpenSLO tooling."},
		{ID: "system-one", Name: "Jev advice", Level: Experimental, Docs: "system-one", Remaining: "Real-provider acceptance and calibration evidence."},
		{ID: "victoriametrics", Name: "VictoriaMetrics", Level: Experimental, Docs: "provider-adapters", Provider: "victoriametrics", Remaining: "Interval and throughput acceptance on a real cluster, including MetricsQL.", Gated: true},
		{ID: "datadog", Name: "Datadog", Level: Experimental, Docs: "provider-adapters", Provider: "datadog", Remaining: "Opt-in live-account acceptance for collection and native publication.", Gated: true},
		{ID: "newrelic", Name: "New Relic", Level: Experimental, Docs: "new-relic", Provider: "newrelic", Remaining: "Opt-in live-account acceptance for collection and native publication.", Gated: true},
		{ID: "dash0", Name: "Dash0", Level: Experimental, Docs: "dash0", Provider: "dash0", Remaining: "Opt-in live-account acceptance, including sessions and interval SLOs.", Gated: true},
		{ID: "googlecloud", Name: "Google Cloud Monitoring", Level: Experimental, Docs: "google-cloud", Provider: "googlecloud", Remaining: "Opt-in live-project acceptance for collection and native publication.", Gated: true},
	}
}

// Lookup returns the feature with the given ID.
func Lookup(id string) (Feature, bool) {
	for _, f := range Catalog() {
		if f.ID == id {
			return f, true
		}
	}
	return Feature{}, false
}

// Document is the published form shared by the CLI and the portal.
type Document struct {
	Levels   []Definition `json:"levels"`
	Features []Feature    `json:"features"`
}

// Published returns the catalog as the CLI and portal present it.
func Published() Document { return Document{Levels: Levels(), Features: Catalog()} }
