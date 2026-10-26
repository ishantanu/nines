package intenteval

import (
	"context"
	"math"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

var templateProbes = []string{"intent-plan", "compatible", "missing", "wrong-type", "incomplete-population", "event-ratio", "no-data", "no-traffic"}

type templateTelemetry struct {
	testtelemetry.Source
	measurement v1.MeasurementSpec
	mode        string
}

func (s *templateTelemetry) Discover(_ context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	m := s.measurement
	d := v1.TelemetryDiscovery{Metric: metric, From: at.Add(-15 * time.Minute), To: at, Metadata: []v1.TelemetryMetadata{{Type: "counter"}}}
	series := func(name string, extra map[string]string) map[string]string {
		row := map[string]string{"__name__": name}
		for k, v := range labels {
			row[k] = v
		}
		for k, v := range extra {
			row[k] = v
		}
		return row
	}
	if m.Type == "latency" {
		d.Metadata = []v1.TelemetryMetadata{{Type: "histogram", Unit: "seconds"}}
		d.Series = []map[string]string{series(metric+"_count", nil), series(metric+"_bucket", map[string]string{"le": "+Inf"})}
		if s.mode != "incomplete-population" {
			d.Series = append(d.Series, series(metric+"_bucket", map[string]string{"le": strconv.FormatFloat(*m.ThresholdSeconds, 'g', -1, 64)}))
		}
	} else {
		unknown := map[string]string{}
		for k := range m.SuccessLabels {
			unknown[k] = "unknown"
		}
		d.Series = []map[string]string{series(metric, unknown)}
		if s.mode != "incomplete-population" {
			d.Series = append(d.Series, series(metric, m.SuccessLabels))
		}
	}
	if s.mode == "missing" {
		d.Series = nil
	}
	if s.mode == "wrong-type" {
		d.Metadata[0].Type = "gauge"
	}
	return d, nil
}

// Templates verifies measurement/compiler contracts, not a model's judgment or
// the correctness of a user's evaluator. It never contacts a configured provider.
func Templates(ctx context.Context) (Report, error) {
	catalog, err := slotemplates.List("")
	report := NewReport("deterministic", "")
	report.Suite = "templates"
	report.SuiteSHA256 = model.DefinitionDigest(struct {
		Catalog string
		Probes  []string
	}{catalog.Digest, templateProbes})
	if err != nil {
		return report, err
	}
	for _, template := range catalog.Templates {
		for _, probe := range templateProbes {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			started := time.Now()
			result := Result{Case: template.ID + "/" + probe, Provider: "deterministic", Model: "none", Status: "contract_checked"}
			result.Failure = checkTemplate(ctx, template, probe)
			result.Passed = result.Failure == ""
			result.DurationMilliseconds = time.Since(started).Milliseconds()
			report.Add(result)
		}
	}
	report.Complete = true
	return report, Summary(report)
}
func checkTemplate(ctx context.Context, template v1.SLOTemplate, probe string) string {
	draft, err := slotemplates.Render(template.ID, v1.TemplateValues{})
	if err != nil || draft.TelemetryVerified || draft.Manifest.Spec.Lifecycle != v1.LifecycleExperimental {
		return "invalid_template_draft"
	}
	manifest := draft.Manifest
	source := &templateTelemetry{measurement: *manifest.Spec.Measurement, mode: probe}
	memory := store.NewMemory()
	svc := service.New(memory, source)
	svc.TenantID = "evaluation"
	compiled, compileErr := svc.CompileMeasurement(ctx, manifest)
	switch probe {
	case "missing", "wrong-type", "incomplete-population":
		if compileErr == nil {
			return "unsupported_telemetry_accepted"
		}
	default:
		if compileErr != nil || compiled.Digest != draft.ManifestDigest || compiled.Manifest.Spec.MeasurementBinding != nil {
			return "compatible_measurement_rejected"
		}
	}
	if probe == "intent-plan" {
		goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: manifest.Metadata, Spec: v1.IntentSpec{Description: template.EligibleEvents, Service: manifest.Spec.Service, Source: manifest.Spec.Source, Outcome: manifest.Spec.Measurement.Type, Population: manifest.Spec.Measurement.Labels, Target: manifest.Spec.Target, WindowSeconds: manifest.Spec.WindowSeconds, StepSeconds: manifest.Spec.StepSeconds, Measurement: manifest.Spec.Measurement, NoDataPolicy: "unknown", Lifecycle: manifest.Spec.Lifecycle}}
		if goal.Spec.Outcome == "latency" {
			goal.Spec.ThresholdSeconds = manifest.Spec.Measurement.ThresholdSeconds
		}
		plan, err := svc.PlanIntent(ctx, v1.PlanIntentInput{Intent: goal})
		if err != nil || plan.Status != "ready" || plan.Compiled == nil || plan.Compiled.Manifest.Spec.Measurement == nil || plan.Compiled.Manifest.Spec.Source != manifest.Spec.Source || plan.Compiled.Manifest.Spec.Lifecycle != v1.LifecycleExperimental {
			return "intent_measurement_plan_failed"
		}
	}
	rows, err := memory.List(ctx)
	if err != nil || len(rows) != 0 {
		return "unexpected_resource_write"
	}
	if probe != "event-ratio" && probe != "no-data" && probe != "no-traffic" {
		return ""
	}
	slo := model.SLODefinition(manifest).SLO()
	slo.Revision = 1
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	observation := model.WindowObservation{Time: at, WindowSeconds: slo.WindowSeconds, Revision: 1, Good: 97, Total: 100, Coverage: 1}
	if probe == "no-traffic" {
		observation.Good = 0
		observation.Total = 0
	}
	record := model.Record{SLO: slo, Windows: map[string]model.WindowObservation{}}
	if probe != "no-data" {
		record.Windows[model.WindowKey(slo.WindowSeconds, at)] = observation
	}
	snapshot := engine.EvaluateRecord(record, at, slo.WindowSeconds)
	if probe == "event-ratio" {
		if snapshot.Reliability == nil || math.Abs(*snapshot.Reliability-.97) > 1e-12 || snapshot.Good != 97 || snapshot.Total != 100 || snapshot.Bad != 3 {
			return "incorrect_event_ratio"
		}
	} else if snapshot.Reliability != nil || snapshot.BurnRate != nil || snapshot.BudgetRemaining != nil {
		return "missing_evidence_reported_healthy"
	}
	return ""
}
