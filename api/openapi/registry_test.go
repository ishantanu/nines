package openapi

import (
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/httpapi"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
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
}
