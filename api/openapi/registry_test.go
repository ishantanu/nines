package openapi

import (
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/datasources"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/skills"
)

// generatedTypes are the request and response bodies of documented stable
// operations, by schema name. Types they contain are added automatically.
var generatedTypes = map[string]any{
	// SLOs
	"SLO":                  model.SLO{},
	"SLOView":              service.SLOView{},
	"Snapshot":             model.Snapshot{},
	"Point":                model.Point{},
	"History":              engine.History{},
	"CustomerReport":       v1.CustomerReport{},
	"ErrorBudgetDecision":  v1.ErrorBudgetDecision{},
	"RetirementPlan":       v1.RetirementPlan{},
	"RetirementInput":      v1.RetirementInput{},
	"RetirementReceipt":    v1.RetirementReceipt{},
	"RulePlan":             rules.Plan{},
	"RuleDeployment":       rules.Deployment{},
	"RuleChange":           httpapi.RuleChange{},
	"RuleAutomationChange": httpapi.RuleAutomationChange{},
	"RuleAutomationResult": v1.RuleAutomationResult{},
	"CompiledMeasurement":  v1.CompiledMeasurement{},
	"SLOContext":           v1.SLOContext{},
	"JourneyContext":       v1.JourneyContext{},
	"SLORevisionList":      httpapi.SLORevisionList{},
	"SLORevision":          v1.SLORevision{},
	"JourneyRevision":      model.JourneyRevision{},
	"IngestSamples":        httpapi.IngestSamples{},
	"IngestResult":         httpapi.IngestResult{},
	"CollectRequest":       httpapi.CollectRequest{},
	"CollectResult":        httpapi.CollectResult{},
	"CreatedSLO":           httpapi.CreatedSLO{},

	// Critical user journeys
	"JourneyView":    service.JourneyView{},
	"JourneyPlan":    service.JourneyPlan{},
	"JourneyRequest": httpapi.JourneyRequest{},
	"JourneyRun":     model.JourneyRun{},
	"RunEvidence":    model.RunEvidence{},

	// Measurement bindings and dependency maps
	"TopologyResource":      httpapi.TopologyResource{},
	"BindingHealth":         v1.BindingHealth{},
	"CompileBindingInput":   v1.CompileBindingInput{},
	"CompiledBinding":       v1.CompiledBinding{},
	"DependencyView":        v1.DependencyView{},
	"DependencyComposition": v1.DependencyComposition{},

	// Access and team access
	"AccessView":            httpapi.AccessView{},
	"TeamDirectory":         identity.TeamDirectory{},
	"TeamAccessAuditList":   identity.AuditList{},
	"GrantInput":            identity.GrantInput{},
	"TeamMember":            identity.Member{},
	"TeamInput":             identity.TeamInput{},
	"Team":                  identity.Team{},
	"ServiceAccountInput":   identity.ServiceAccountInput{},
	"CreatedServiceAccount": identity.CreatedServiceAccount{},
	"StatusResult":          identity.StatusResult{},
	"LocalUserInput":        identity.LocalUserInput{},
	"User":                  identity.User{},

	// Telemetry connections
	"ConnectionList":       httpapi.ConnectionList{},
	"ConnectionView":       datasources.ConnectionView{},
	"ConnectionInput":      datasources.Input{},
	"ConnectionTestResult": httpapi.ConnectionTestResult{},
	"ConnectionDeleted":    httpapi.ConnectionDeleted{},
	"MetricNames":          v1.MetricNames{},
	"TelemetryDiscovery":   v1.TelemetryDiscovery{},

	// Platform administration
	"PlatformOverview":    identity.PlatformOverview{},
	"TenantCreateRequest": identity.TenantCreateRequest{},
	"TenantChangeRequest": identity.TenantChangeRequest{},
	"RegisteredTenant":    identity.RegisteredTenant{},
	"TenantCreated":       identity.TenantCreated{},
	"UserCreated":         identity.UserCreated{},
	"SetupLink":           identity.SetupLink{},
	"PlatformUserPage":    identity.PlatformUserPage{},
	"PlatformAuditPage":   identity.PlatformAuditPage{},
	"PlatformAIStatus":    identity.PlatformAIStatus{},
	"PlatformOK":          identity.PlatformOK{},
	"PlatformRequest":     identity.PlatformRequest{},

	// CLI sign-in
	"CLIStartInput":    identity.CLIStartInput{},
	"CLIStarted":       identity.CLIStarted{},
	"CLIExchangeInput": identity.CLIExchangeInput{},
	"CLIToken":         identity.CLIToken{},

	// Audit
	"AuditPage":          v1.AuditPage{},
	"AuditRecord":        v1.AuditRecord{},
	"AuditArchiveStatus": v1.AuditArchiveStatus{},

	// SLO templates
	"SLOTemplateCatalog": v1.SLOTemplateCatalog{},
	"SLOTemplateDetail":  v1.SLOTemplateDetail{},
	"TemplateValues":     v1.TemplateValues{},
	"SLOTemplateDraft":   v1.SLOTemplateDraft{},

	// Workspace, search, topology, skills and alert deliveries
	"Workspace":       httpapi.WorkspaceInfo{},
	"ResourceSearch":  v1.ResourceSearch{},
	"TopologyCatalog": v1.TopologyCatalog{},
	"Skill":           skills.Skill{},
	"AlertDelivery":   model.Delivery{},
}
