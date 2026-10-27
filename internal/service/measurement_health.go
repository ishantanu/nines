package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func (s *Service) inspectBinding(ctx context.Context, ref v1.BindingReference) (v1.BindingHealth, error) {
	out := v1.BindingHealth{APIVersion: v1.APIVersion, Tenant: s.TenantID, Binding: ref, CheckedAt: s.Now().UTC(), State: "unavailable", Code: "telemetry_unavailable", Message: "Measurement evidence is unavailable; the contract has not been revalidated."}
	row, err := s.ReadTopology(ctx, v1.BindingKind, ref.Name, ref.Revision)
	if err != nil {
		return out, err
	}
	if row.Digest != ref.Digest {
		return out, model.Invalid("binding digest does not match its immutable revision")
	}
	var b model.MeasurementBinding
	if err = json.Unmarshal(row.Manifest, &b); err != nil {
		return out, err
	}
	out.SourceDigest = s.SourceFingerprint(b.Spec.Source)
	evidence, err := s.DiscoverTelemetry(ctx, b.Spec.Source, b.Spec.Metric, b.Spec.Labels)
	if err != nil {
		return out, nil
	}
	if s.SourceFingerprint(b.Spec.Source) != out.SourceDigest {
		return out, model.ErrRevisionConflict
	}
	out.Fingerprint = evidence.Fingerprint
	if evidence.Truncated || len(evidence.Metadata) != 1 || len(evidence.Series) == 0 {
		out.State = "unknown"
		out.Code = "insufficient_evidence"
		out.Message = "Discovery evidence is missing, ambiguous or truncated; compatibility cannot be established."
		return out, nil
	}
	if err = verifyBindingEvidence(b, evidence); err != nil {
		out.State = "drifted"
		out.Code = "measurement_contract_mismatch"
		out.Message = err.Error()
		out.EvaluationBlocked = true
		return out, nil
	}
	out.State = "compatible"
	out.Code = "measurement_contract_verified"
	out.Message = "Recent metadata and series match the binding. Traffic, scrape coverage and business success semantics require separate evidence."
	return out, nil
}
func (s *Service) CheckBinding(ctx context.Context, id string, expected int64) (v1.BindingHealth, error) {
	if expected < 1 {
		return v1.BindingHealth{}, model.Invalid("positive expectedRevision is required")
	}
	row, err := s.ReadTopology(ctx, v1.BindingKind, id, 0)
	if err != nil {
		return v1.BindingHealth{}, err
	}
	if row.Revision != expected {
		return v1.BindingHealth{}, model.ErrRevisionConflict
	}
	out, err := s.inspectBinding(ctx, v1.BindingReference{Name: id, Revision: row.Revision, Digest: row.Digest})
	if err != nil {
		return out, err
	}
	latest, err := s.ReadTopology(ctx, v1.BindingKind, id, 0)
	if err != nil {
		return out, err
	}
	if latest.Revision != expected {
		return out, model.ErrRevisionConflict
	}
	return out, nil
}
func (s *Service) measurementHealth(record model.Record) (v1.BindingHealth, bool) {
	check := record.Collection.Measurement
	if check == nil || check.SLORevision != model.ConfigRevision(record.SLO) || (record.SLO.Measurement != nil && check.MeasurementDigest != MeasurementDigest(record.SLO)) || (record.SLO.Measurement == nil && (record.SLO.MeasurementBinding == nil || model.DefinitionDigest(check.Binding) != model.DefinitionDigest(*record.SLO.MeasurementBinding))) {
		return v1.BindingHealth{}, false
	}
	health := *check
	if health.SourceDigest == "" || health.SourceDigest != s.SourceFingerprint(record.SLO.Source) {
		health.State = "stale"
		health.Code = "measurement_source_changed"
		health.Message = "The source route changed since this measurement check. Revalidate the measurement against the current source."
		health.EvaluationBlocked = true
		return health, true
	}
	if health.State == "compatible" && s.Now().Sub(health.CheckedAt) > 10*time.Minute {
		health.State = "stale"
		health.Code = "measurement_check_overdue"
		health.Message = "Measurement contract check is overdue."
	}
	return health, true
}

// CheckMeasurement persists revision-bound operational health in the existing
// collection-health document, without changing definitions or stored samples.
func (s *Service) CheckMeasurement(ctx context.Context, id string) (v1.BindingHealth, error) {
	record, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.BindingHealth{}, err
	}
	config := record.SLO
	if config.Retired() {
		return v1.BindingHealth{}, model.Invalid("SLO is archived")
	}
	if config.MeasurementBinding == nil && config.Measurement == nil {
		return v1.BindingHealth{}, model.Invalid("SLO has no structured measurement")
	}
	var out v1.BindingHealth
	if config.Measurement != nil {
		out, err = s.inspectInlineMeasurement(ctx, config)
	} else {
		out, err = s.inspectBinding(ctx, *config.MeasurementBinding)
	}
	if err != nil {
		return out, err
	}
	out.SLORevision = model.ConfigRevision(config)
	err = s.Store.Update(ctx, id, func(r *model.Record) error {
		if model.ConfigRevision(r.SLO) != model.ConfigRevision(config) {
			return model.ErrRevisionConflict
		}
		if model.DefinitionDigest(r.Collection.Measurement) != model.DefinitionDigest(record.Collection.Measurement) {
			return model.ErrRevisionConflict
		}
		previous, ok := s.measurementHealth(*r)
		if ok && previous.EvaluationBlocked && out.State != "compatible" {
			out.EvaluationBlocked = true
		}
		r.Collection.Measurement = &out
		return nil
	})
	return out, err
}
func (s *Service) RunMeasurementMonitor(ctx context.Context, log *slog.Logger) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		pass, cancel := context.WithTimeout(ctx, 30*time.Second)
		configs, err := s.Store.List(pass)
		if err == nil {
			checked := 0
			for _, c := range configs {
				if c.Retired() {
					continue
				}
				if c.MeasurementBinding == nil && c.Measurement == nil {
					continue
				}
				record, readErr := s.Store.Read(pass, c.ID)
				if readErr != nil {
					continue
				}
				previous, ok := s.measurementHealth(record)
				if ok && s.Now().Sub(previous.CheckedAt) < 5*time.Minute {
					continue
				}
				job, stop := context.WithTimeout(pass, 8*time.Second)
				health, err := s.CheckMeasurement(job, c.ID)
				stop()
				checked++
				if err != nil {
					log.Warn("Measurement contract check failed", "slo", c.ID)
				} else if health.State != "compatible" {
					log.Warn("Measurement contract requires attention", "slo", c.ID, "state", health.State, "code", health.Code)
				}
				if checked >= 20 || pass.Err() != nil {
					break
				}
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Service) inspectInlineMeasurement(ctx context.Context, c model.SLO) (v1.BindingHealth, error) {
	out := v1.BindingHealth{APIVersion: v1.APIVersion, Tenant: s.TenantID, MeasurementDigest: MeasurementDigest(c), SourceDigest: s.SourceFingerprint(c.Source), CheckedAt: s.Now().UTC(), State: "unavailable", Code: "telemetry_unavailable", Message: "Measurement evidence is unavailable."}
	evidence, err := s.DiscoverTelemetry(ctx, sourceID(c.Source), c.Measurement.Metric, c.Measurement.Labels)
	if err != nil {
		return out, nil
	}
	if s.SourceFingerprint(c.Source) != out.SourceDigest {
		return out, model.ErrRevisionConflict
	}
	out.Fingerprint = evidence.Fingerprint
	if evidence.Truncated || len(evidence.Series) == 0 || len(evidence.Metadata) != 1 {
		out.State = "unknown"
		out.Code = "insufficient_evidence"
		out.Message = "Recent telemetry does not establish the measurement contract."
		return out, nil
	}
	if err = verifyBindingEvidence(model.InlineBinding(c), evidence); err != nil {
		out.State = "drifted"
		out.Code = "measurement_contract_mismatch"
		out.Message = err.Error()
		out.EvaluationBlocked = true
		return out, nil
	}
	out.State = "compatible"
	out.Code = "measurement_contract_verified"
	out.Message = "Recent telemetry matches the SLO-owned measurement. Coverage and business semantics require separate evidence."
	return out, nil
}
