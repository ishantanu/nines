package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

// IntentSourceDigest binds intent receipts to a tenant and configured route without exposing route identity.
func (s *Service) IntentSourceDigest(source string) string {
	return model.DefinitionDigest(struct{ Tenant, Source, Route string }{s.TenantID, source, s.SourceFingerprint(source)})
}
func validateEvidenceHistory(h *v1.IntentEvidenceRange, step int64, latest time.Time) error {
	if h == nil {
		return nil
	}
	if h.From.IsZero() || h.To.IsZero() || !h.To.After(h.From) || h.To.Sub(h.From) > 72*time.Hour || h.ResolutionSeconds < 1 || h.ResolutionSeconds > 3600 || h.To.Sub(h.From)%(time.Duration(h.ResolutionSeconds)*time.Second) != 0 || h.To.Sub(h.From)/(time.Duration(h.ResolutionSeconds)*time.Second) > 240 || !aligned(h.From, step) || !aligned(h.To, step) || h.ResolutionSeconds%step != 0 || h.To.After(latest) {
		return model.Invalid("history requires an explicit aligned completed range up to 72h and 240 cutoffs, at 1–3600 second resolution")
	}
	return nil
}
func evidenceVariant(goal v1.SLOIntent, a v1.IntentEvidenceAlternative) (v1.SLOIntent, error) {
	if a.Target != nil {
		goal.Spec.Target = *a.Target
	}
	if len(a.Window) > 32 {
		return goal, model.Invalid("comparison window exceeds 32 bytes")
	}
	if a.Window != "" {
		d, err := time.ParseDuration(a.Window)
		if err != nil || d <= 0 || d > 31*24*time.Hour || d%time.Second != 0 {
			return goal, model.Invalid("comparison window needs whole-second duration units within 31 days")
		}
		goal.Spec.WindowSeconds = int64(d / time.Second)
	}
	if a.Policies != nil {
		goal.Spec.Policy = nil
		goal.Spec.Policies = *a.Policies
	}
	return goal, model.ValidateIntent(goal)
}

// PrepareIntentEvidence reads a complete pinned inline measurement. It does not
// confirm semantics, choose a target, save evidence or apply a definition.
func (s *Service) PrepareIntentEvidence(ctx context.Context, in v1.IntentEvidenceInput, templates *slotemplates.Registry) (v1.IntentEvidence, error) {
	var out v1.IntentEvidence
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	body, err := json.Marshal(in)
	if err != nil || len(body) > 32<<10 {
		return out, model.Invalid("evidence request exceeds 32 KiB")
	}
	// Detach maps/pointers before giving measurement configuration to adapters.
	var detached v1.IntentEvidenceInput
	if err = json.Unmarshal(body, &detached); err != nil {
		return out, model.Invalid("invalid evidence input")
	}
	in = detached
	if in.Plan.Intent.Spec.Measurement == nil || in.Plan.Binding != nil || in.ExpectedRevision < 0 || len(in.Alternatives) > 2 {
		return out, model.Invalid("provide a complete inline measurement, nonnegative expectedRevision and at most two alternatives")
	}
	if err = model.ValidateIntent(in.Plan.Intent); err != nil {
		return out, err
	}
	if in.Plan.Intent.Spec.Source != "primary" {
		_, found := s.Sources[in.Plan.Intent.Spec.Source]
		if !found && s.DynamicSource != nil {
			_, found = s.DynamicSource(in.Plan.Intent.Spec.Source)
		}
		if !found {
			return out, model.ErrNotFound
		}
	}
	config := model.SLODefinition(inlineIntentManifest(in.Plan.Intent)).SLO()
	if err = model.SLODefinition(inlineIntentManifest(in.Plan.Intent)).Validate(); err != nil {
		return out, err
	}
	// First reviews capture current provenance. Explicit pins from an earlier
	// candidate/plan must match; omitting them is never human confirmation.
	if in.IntentDigest == "" {
		in.IntentDigest = model.DefinitionDigest(in.Plan.Intent)
	}
	if in.MeasurementDigest == "" {
		in.MeasurementDigest = MeasurementDigest(config)
	}
	if in.SourceDigest == "" {
		in.SourceDigest = s.IntentSourceDigest(config.Source)
	}
	if in.IntentDigest != model.DefinitionDigest(in.Plan.Intent) || in.MeasurementDigest != MeasurementDigest(config) || in.SourceDigest != s.IntentSourceDigest(config.Source) {
		return out, model.ErrRevisionConflict
	}
	catalog, err := templates.List("")
	if err != nil {
		return out, err
	}
	if in.CatalogDigest == "" {
		in.CatalogDigest = catalog.Digest
	}
	if in.CatalogDigest != catalog.Digest {
		return out, model.ErrRevisionConflict
	}
	route := s.SourceFingerprint(config.Source)
	fence := func() error {
		if s.SourceFingerprint(config.Source) != route {
			return model.ErrRevisionConflict
		}
		current, err := s.Store.Read(ctx, config.ID)
		if errors.Is(err, model.ErrNotFound) {
			if in.ExpectedRevision == 0 {
				return nil
			}
			return model.ErrRevisionConflict
		}
		if err != nil {
			return err
		}
		if model.ConfigRevision(current.SLO) != in.ExpectedRevision {
			return model.ErrRevisionConflict
		}
		return nil
	}
	if err = fence(); err != nil {
		return out, err
	}
	latest := s.evaluationAt(config)
	if err = validateEvidenceHistory(in.History, config.StepSeconds, latest); err != nil {
		return out, err
	}
	goals := []v1.SLOIntent{in.Plan.Intent}
	names := []string{"baseline"}
	seen := map[string]bool{"baseline": true}
	for _, a := range in.Alternatives {
		if !validResourceID(a.Name) || seen[a.Name] {
			return out, model.Invalid("comparison names must be distinct IDs other than baseline")
		}
		seen[a.Name] = true
		goal, err := evidenceVariant(in.Plan.Intent, a)
		if err != nil {
			return out, err
		}
		if err = model.SLODefinition(inlineIntentManifest(goal)).Validate(); err != nil {
			return out, err
		}
		goals = append(goals, goal)
		names = append(names, a.Name)
	}
	out = v1.IntentEvidence{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: s.Now().UTC(), IntentDigest: in.IntentDigest, MeasurementDigest: in.MeasurementDigest, SourceDigest: in.SourceDigest, CatalogDigest: in.CatalogDigest, ExpectedRevision: in.ExpectedRevision, State: "observed", Comparisons: []v1.IntentEvidenceComparison{}, MaxAdapterCalls: 1 + len(goals), HumanConfirmationRequired: true, Warnings: []string{
		"read_only_preview_not_confirmation_approval_or_deployment",
		"counts_and_coverage_are_provider_estimates_not_statistical_confidence_or_business_correctness",
		"freshness_describes_returned_cutoffs_not_independent_instrumentation_freshness",
		"overlapping_window_counts_are_never_summed",
		"historical_cutoffs_are_sampled_not_continuous_alert_replay_or_notification_verification",
		"policy_budget_fraction_assumes_constant_eligible_event_rate_not_an_outage_duration_prediction",
		"no_recommended_target_or_automatic_policy_selection",
		"lifecycle_permission_is_not_deployed_rules_provider_evaluation_or_notification_delivery",
		"adapter_calls_are_not_underlying_http_request_counts",
	}}
	if in.History != nil {
		out.MaxAdapterCalls += len(goals)
	}
	// Capability checks happen before upstream metadata reads.
	_, discovery := s.CollectorFor(config).(telemetryprovider.MetricDiscoverySource)
	_, windows := s.CollectorFor(config).(WindowPreviewer)
	if !discovery || !windows {
		out.State = "unsupported"
		out.Warnings = append(out.Warnings, "adapter_requires_metadata_discovery_and_window_preview")
		out.Digest = model.DefinitionDigest(out)
		return out, nil
	}
	out.AdapterCalls++
	job, stop := context.WithTimeout(ctx, 10*time.Second)
	var compileInput v1.IntentEvidenceInput
	if err = json.Unmarshal(body, &compileInput); err != nil {
		stop()
		return out, model.Invalid("invalid evidence input")
	}
	plan, err := s.planIntent(job, compileInput.Plan, true)
	stop()
	if err != nil {
		return out, err
	}
	if plan.Compiled == nil || plan.ExpectedRevision != in.ExpectedRevision || plan.SourceDigest != route || plan.Compiled.MeasurementDigest != in.MeasurementDigest {
		return out, model.ErrRevisionConflict
	}
	probe := v1.IntentMeasurementCandidate{RequestedMetric: config.Measurement.Metric, Metric: config.Measurement.Metric, MetricType: "unknown", Unit: "unknown"}
	if err = readCandidateEvidence(&probe, plan.Compiled.Evidence, v1.IntentCandidateInput{Population: in.Plan.Intent.Spec.Population}, s.Now().UTC(), nil); err != nil {
		return out, ErrTelemetry
	}
	for i, goal := range goals {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if err = fence(); err != nil {
			return out, err
		}
		variant := plan
		manifest := inlineIntentManifest(goal)
		compiled := *plan.Compiled
		compiled.Manifest = manifest
		compiled.Digest = model.DefinitionDigest(manifest)
		variant.Compiled = &compiled
		variant.Intent = goal
		variant.IntentDigest = model.DefinitionDigest(goal)
		c := model.SLODefinition(manifest).SLO()
		c.Revision = 1
		row := v1.IntentEvidenceComparison{LifecycleAllowsAlerts: c.AlertsEnabled(), Name: names[i], State: "observed", Target: c.Target, WindowSeconds: c.WindowSeconds, AllowedBadEventsPerMillion: 1e6 * (1 - c.Target), Policies: []v1.IntentPolicyTradeoff{}, HistoryState: "not_requested", Gaps: []string{}}
		if c.Quality != nil && len(c.Quality.CohortLabels) > 0 {
			row.State = "insufficient_evidence"
			row.Gaps = append(row.Gaps, "configured_cohorts_not_inspected")
			out.State = "partial"
		}
		for _, p := range c.AlertPolicies() {
			row.Policies = append(row.Policies, v1.IntentPolicyTradeoff{MinCoverage: p.MinCoverage, MinEvents: p.MinEvents, Confidence: p.Confidence, Name: p.Name, Severity: p.Severity, ShortWindowSeconds: p.ShortWindowSeconds, LongWindowSeconds: p.LongWindowSeconds, ForSeconds: p.ForSeconds, BurnRate: p.BurnRate, LongWindowBudgetFraction: p.BurnRate * float64(p.LongWindowSeconds) / float64(c.WindowSeconds)})
		}
		out.AdapterCalls++
		job, stop = context.WithTimeout(ctx, 10*time.Second)
		r, readErr := s.readIntentWindows(job, c, latest.Add(-5*time.Duration(c.StepSeconds)*time.Second), latest, c.StepSeconds)
		stop()
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err = fence(); err != nil {
			return out, err
		}
		if readErr != nil {
			row.State = "provider_error"
			row.Gaps = append(row.Gaps, "current_window_read_failed_or_invalid_receipt")
			out.State = "partial"
		} else {
			at := latest
			found := false
			for _, o := range r.Windows {
				if o.WindowSeconds == c.WindowSeconds && (!found || o.Time.After(at)) {
					at = o.Time
					found = true
				}
			}
			x := engine.EvaluateRecord(r, at, c.WindowSeconds)
			r.Collection = v1.CollectionHealth{SourceDigest: route, SourceFrom: x.From, SourceTo: at, Measurement: &v1.BindingHealth{State: "compatible", SourceDigest: route, SLORevision: 1, MeasurementDigest: in.MeasurementDigest, CheckedAt: out.ObservedAt}}
			q := s.qualityFromRecord(r, x, out.ObservedAt, false)
			row.Current = &v1.IntentEvidenceCurrent{State: q.State, At: x.At, From: x.From, Good: x.Good, Total: x.Total, Coverage: x.Coverage, Reliability: x.Reliability, BurnRate: x.BurnRate, DataStatus: x.DataStatus, Freshness: q.Freshness, MinimumEvents: q.MinimumEvents, Checks: q.Checks}
			if q.State != "usable" {
				row.State = "insufficient_evidence"
				out.State = "partial"
				row.Gaps = append(row.Gaps, "review_current_quality_checks")
			}
		}
		if in.History != nil {
			out.AdapterCalls++
			h := in.History
			job, stop = context.WithTimeout(ctx, 10*time.Second)
			history, e := s.backtestCompiledIntent(job, v1.IntentBacktestInput{Plan: v1.PlanIntentInput{Intent: goal}, From: h.From, To: h.To, ResolutionSeconds: h.ResolutionSeconds}, variant, v1.IntentBacktest{})
			stop()
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if err = fence(); err != nil {
				return out, err
			}
			if e != nil {
				row.HistoryState = "provider_error"
				out.State = "partial"
				row.Gaps = append(row.Gaps, "historical_read_failed_or_invalid_receipt")
			} else {
				row.HistoryState = "sampled"
				row.History = &v1.IntentEvidenceHistory{From: history.From, To: history.To, ResolutionSeconds: history.ResolutionSeconds, CompleteCutoffs: history.CompleteCutoffs, UnknownCutoffs: history.UnknownCutoffs, NoTrafficCutoffs: history.NoTrafficCutoffs, TargetViolationCutoffs: history.TargetViolationCutoffs, BurnThresholdCutoffs: history.BurnThresholdCutoffs}
				if history.UnknownCutoffs > 0 || history.NoTrafficCutoffs > 0 {
					row.Gaps = append(row.Gaps, "sampled_history_contains_unknown_or_no_traffic_cutoffs")
					out.State = "partial"
				}
			}
		}
		out.Comparisons = append(out.Comparisons, row)
	}
	if err = fence(); err != nil {
		return out, err
	}
	if ref := in.Plan.Intent.Spec.Journey; ref != nil {
		r, err := s.Journeys.JourneyRevision(ctx, ref.Name, 0)
		if err != nil {
			return out, err
		}
		if r.Revision != ref.ExpectedRevision {
			return out, model.ErrRevisionConflict
		}
	}
	out.Digest = model.DefinitionDigest(out)
	body, err = json.Marshal(out)
	if err != nil || len(body) > 128<<10 {
		return v1.IntentEvidence{}, model.Invalid("evidence report exceeds 128 KiB")
	}
	return out, nil
}
