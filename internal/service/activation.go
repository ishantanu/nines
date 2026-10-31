package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// IntegrationStatus only inspects trusted destinations; it never deploys rules.
func (s *Service) IntegrationStatus(ctx context.Context, id string) (v1.IntegrationStatus, error) {
	record, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.IntegrationStatus{}, err
	}
	now := s.Now().UTC()
	out := v1.IntegrationStatus{APIVersion: model.APIVersion, Kind: "SLOIntegrationStatus", Tenant: s.TenantID, SLOID: record.SLO.ID, Revision: model.ConfigRevision(record.SLO), CheckedAt: now,
		Definition: v1.IntegrationStage{State: "saved", Message: fmt.Sprintf("Definition revision %d is saved in Nines.", model.ConfigRevision(record.SLO))},
		Collection: v1.IntegrationStage{State: "not_collected", Code: "collection_not_observed", Message: "No successful provider collection has been observed for this revision.", NextAction: "Run collection for the current revision or check its collection schedule. Rule activation alone does not calculate SLO health."},
		Deployment: v1.IntegrationStage{State: "not_configured", Code: "destination_not_configured", Message: "Configure a rule destination to deploy recording and alert rules.", NextAction: "Ask a tenant owner to configure the intended shared Prometheus directory or Mimir namespace, then review a rule plan."},
		Evaluation: v1.IntegrationStage{State: "unknown", Code: "evaluator_not_configured", Message: "External rule evaluation has not been verified.", NextAction: "Configure an evaluator destination before checking loaded rules. Collection does not verify external evaluation."},
	}
	h := record.Collection
	if s.CollectorFor(record.SLO) == nil || !record.SLO.HasQueries() {
		out.Collection = v1.IntegrationStage{State: "not_configured", Code: "collection_not_configured", Message: "This SLO has no configured provider collector and queries.", NextAction: "Configure a query source and good/total measurement queries. Rule files do not ingest application metrics."}
	} else if h.State == "failed" {
		out.Collection = v1.IntegrationStage{State: "failed", Code: "collection_failed", Message: "The latest collection attempt failed.", NextAction: "Test the query connection and inspect collection logs, query permissions and the configured tenant."}
	} else if h.SLORevision == out.Revision && h.State == "succeeded" {
		out.Collection = v1.IntegrationStage{State: "empty", Code: "collection_empty", Message: "The provider query succeeded but returned no measurement intervals.", NextAction: "Verify application metrics exist in the selected dataset and both queries return matching intervals. Allow enough coverage for the selected window.", ObservedAt: &h.CompletedAt}
		if h.Points > 0 {
			out.Collection.State = "collected"
			out.Collection.Code = "measurements_collected"
			out.Collection.NextAction = "Inspect SLI coverage and the SLO evaluation window; collection and external rules are independent."
			out.Collection.Message = fmt.Sprintf("The latest collection stored %d measurements. This does not deploy external rules.", h.Points)
			limit := max(45*time.Second, 2*time.Duration(record.SLO.StepSeconds)*time.Second)
			if h.CompletedAt.After(now) || now.Sub(h.CompletedAt) > limit || h.To.After(now) || now.Sub(h.To) > limit+45*time.Second {
				out.Collection.State = "stale"
				out.Collection.Code = "collection_stale"
				out.Collection.NextAction = "Check collection scheduling and provider timestamps, then collect a fresh range for this revision."
				out.Collection.Message = "Measurements were collected, but the latest provider range is stale."
			}
		}
	}
	output := v1.IntegrationStage{State: "unknown", Code: "rule_output_not_configured", Message: "Recorded output has not been verified through the query source.", NextAction: "Configure a supported query source and reviewed rules. Runtime evaluation and query-dataset output are separate evidence."}
	if s.RuleManager != nil {
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		fingerprint := s.SourceFingerprint(record.SLO.Source)
		var checks sync.WaitGroup
		checks.Go(func() { out.Deployment, out.Evaluation = s.RuleManager.Inspect(ctx, record.SLO, now) })
		if identity, err := s.RuleManager.OutputIdentity(ctx, record.SLO); err == nil {
			if query, ok := s.CollectorFor(record.SLO).(interface {
				RuleOutput(context.Context, model.RuleOutputIdentity, time.Time) v1.IntegrationStage
			}); ok {
				checks.Go(func() { output = query.RuleOutput(ctx, identity, now) })
			}
		} else {
			output = v1.IntegrationStage{State: "unknown", Code: "rule_output_identity_unverified", Message: "The current rule publication identity could not be verified.", NextAction: "Inspect definition and deployment evidence. Output verification requires the current revision and resource owner; no older identity is reused."}
		}
		checks.Wait()
		if fingerprint != s.SourceFingerprint(record.SLO.Source) {
			output = v1.IntegrationStage{State: "unknown", Code: "rule_output_source_changed", Message: "The query source changed during verification.", NextAction: "Refresh integration status against the current source before relying on the evidence."}
		}
	}
	native := false
	if s.RuleManager != nil {
		target, ok := s.RuleManager.TargetFor(record.SLO)
		native = ok && (target.Kind == "dash0" || target.Kind == "googlecloud" || target.Kind == "datadog" || target.Kind == "newrelic")
	}
	if native {
		output = v1.IntegrationStage{State: "unsupported", Code: "native_slo_no_recordings", Message: "Native SLO publication does not install Nines recording rules.", NextAction: "Inspect collected Nines SLI and native provider evaluation separately."}
	}
	if capability, ok := s.CollectorFor(record.SLO).(interface{ SupportsRuleDeployment() bool }); ok && !capability.SupportsRuleDeployment() && !native {
		stage := v1.IntegrationStage{State: "unsupported", Code: "provider_rules_unsupported", Message: "This provider collects measurements; external rule deployment and verification are not supported.", NextAction: "Inspect collected SLI data and Nines burn alerts. Configure provider-native monitors separately."}
		out.Deployment = stage
		out.Evaluation = stage
		output = stage
	}
	out.Automation = s.RuleAutomationStatus(record, s.RuleManager)
	out.RuleOutput = &output
	return out, nil
}
