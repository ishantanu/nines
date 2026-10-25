package service

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

type WindowPreviewer interface {
	CollectWindowRange(context.Context, model.SLO, time.Time, time.Time, int64) ([]model.WindowObservation, error)
}

// BacktestIntent performs bounded, read-only historical sampling with the exact
// runtime evaluator. It does not persist samples, replay alerts or deploy rules.
func (s *Service) BacktestIntent(ctx context.Context, in v1.IntentBacktestInput) (out v1.IntentBacktest, err error) {
	ctx, finish := observability.Operation(ctx, "intent.backtest", s.TenantID)
	defer func() { finish(err) }()
	out = v1.IntentBacktest{APIVersion: v1.APIVersion, Tenant: s.TenantID, Points: []v1.IntentBacktestPoint{}, Warnings: []string{"Each point evaluates a full rolling window independently. Overlapping event counts are never summed.", "Sampled burn threshold crossings do not simulate pending duration, notification delivery or every intervening cutoff.", "Historical queries use the reviewed measurement contract. Retention, scrape estimates and missing instrumentation limit the evidence; this is not a proof of business semantics."}}
	if in.Plan.Binding == nil && in.Plan.Intent.Spec.Measurement == nil {
		return out, model.Invalid("backtesting requires an explicitly selected binding revision")
	}
	if in.From.IsZero() || in.To.IsZero() || !in.To.After(in.From) || in.To.Sub(in.From) > 72*time.Hour {
		return out, model.Invalid("backtest range must be positive and at most 72 hours")
	}
	if in.ResolutionSeconds < 1 || in.ResolutionSeconds > 3600 || in.To.Sub(in.From)%(time.Duration(in.ResolutionSeconds)*time.Second) != 0 || in.To.Sub(in.From)/(time.Duration(in.ResolutionSeconds)*time.Second) > 240 {
		return out, model.Invalid("resolution must be 1–3600 seconds, divide the range and produce at most 240 cutoffs")
	}
	plan, err := s.planIntent(ctx, in.Plan, false)
	if err != nil {
		return out, err
	}
	if plan.Compiled == nil {
		return out, model.Invalid("intent must compile before backtesting")
	}
	def := model.SLODefinition(plan.Compiled.Manifest)
	config := def.SLO()
	config.Revision = 1
	if !config.AlertsEnabled() {
		out.Warnings = append(out.Warnings, "This lifecycle disables burn alerts. Threshold crossings below are measurement previews, not alert episodes or notifications.")
	}
	if !aligned(in.From, config.StepSeconds) || !aligned(in.To, config.StepSeconds) || in.ResolutionSeconds%config.StepSeconds != 0 || in.To.After(s.evaluationAt(config)) {
		return out, model.Invalid("range and resolution must align to stepSeconds and end at a completed provider cutoff")
	}
	collector, ok := s.CollectorFor(config).(WindowPreviewer)
	if !ok {
		return out, model.ErrUnavailable
	}
	observations, err := collector.CollectWindowRange(ctx, config, in.From, in.To, in.ResolutionSeconds)
	if err != nil {
		return out, ErrTelemetry
	}
	out.From = in.From.UTC()
	out.To = in.To.UTC()
	out.ResolutionSeconds = in.ResolutionSeconds
	out.Binding = plan.Compiled.Binding
	out.SourceDigest = plan.SourceDigest
	out.IntentDigest = plan.IntentDigest
	out.ManifestDigest = plan.Compiled.Digest
	r := model.Record{SLO: config, Windows: map[string]model.WindowObservation{}}
	allowed := map[int64]bool{}
	for _, w := range config.Windows() {
		allowed[w] = true
	}
	if len(observations) > 240*len(allowed) {
		return out, model.Invalid("provider returned too many backtest samples")
	}
	for _, o := range observations {
		if err := o.Validate(config); err != nil {
			return out, ErrTelemetry
		}
		key := model.WindowKey(o.WindowSeconds, o.Time)
		if !o.Time.After(in.From) || o.Time.After(in.To) || o.Time.Sub(in.From)%(time.Duration(in.ResolutionSeconds)*time.Second) != 0 || !allowed[o.WindowSeconds] {
			return out, ErrTelemetry
		}
		if _, exists := r.Windows[key]; exists {
			return out, ErrTelemetry
		}
		r.Windows[key] = o
	}
	for at := in.From.Add(time.Duration(in.ResolutionSeconds) * time.Second); !at.After(in.To); at = at.Add(time.Duration(in.ResolutionSeconds) * time.Second) {
		snapshot := engine.EvaluateRecord(r, at, config.WindowSeconds)
		p := v1.IntentBacktestPoint{At: at.UTC(), Reliability: snapshot.Reliability, Coverage: snapshot.Coverage, DataStatus: snapshot.DataStatus}
		if snapshot.Coverage == 1 && snapshot.Reliability != nil {
			out.CompleteCutoffs++
			p.TargetViolated = *snapshot.Reliability < config.Target
			if p.TargetViolated {
				out.TargetViolationCutoffs++
			}
		} else if snapshot.Coverage == 1 && snapshot.DataStatus == "no_traffic" {
			out.NoTrafficCutoffs++
		} else {
			out.UnknownCutoffs++
		}
		for _, policy := range config.AlertPolicies() {
			short := engine.EvaluateRecord(r, at, policy.ShortWindowSeconds)
			long := engine.EvaluateRecord(r, at, policy.LongWindowSeconds)
			p.ShortCoverage = short.Coverage
			p.LongCoverage = long.Coverage
			p.ShortBurn = short.BurnRate
			p.LongBurn = long.BurnRate
			breach := short.Coverage >= policy.MinCoverage && long.Coverage >= policy.MinCoverage && short.Total >= policy.MinEvents && long.Total >= policy.MinEvents && short.BurnRate != nil && long.BurnRate != nil && *short.BurnRate >= policy.BurnRate && *long.BurnRate >= policy.BurnRate
			if breach {
				p.BurnThresholdExceeded = true
				if policy.Name != "" {
					p.PolicyBreaches = append(p.PolicyBreaches, policy.Name)
				}
			}
		}
		if p.BurnThresholdExceeded {
			out.BurnThresholdCutoffs++
		}
		out.Points = append(out.Points, p)
	}
	if in.Plan.Binding != nil {
		latest, e := s.ReadTopology(ctx, v1.BindingKind, in.Plan.Binding.Name, 0)
		if e != nil {
			return out, e
		}
		if latest.Revision != in.Plan.Binding.ExpectedRevision {
			return out, model.ErrRevisionConflict
		}
	}
	if s.SourceFingerprint(in.Plan.Intent.Spec.Source) != plan.SourceDigest {
		return out, model.ErrRevisionConflict
	}
	return out, nil
}
