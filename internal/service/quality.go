package service

import (
	"context"
	"math"
	"sort"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// CohortSource is optional. Adapters return whole-window counts at one cutoff;
// query execution and provider credentials remain adapter concerns.
type CohortSource interface {
	ReadCohorts(context.Context, model.SLO, []string, time.Time) (v1.CohortEvidence, error)
}

func (s *Service) qualityFromRecord(r model.Record, x model.Snapshot, now time.Time, historical bool) v1.MeasurementQuality {
	c := r.SLO
	q := v1.MeasurementQuality{APIVersion: v1.APIVersion, Tenant: s.TenantID, SLOID: c.ID, Revision: model.ConfigRevision(c), DefinitionDigest: model.DefinitionDigest(model.DefinitionFromSLO(c)), SourceDigest: s.SourceFingerprint(c.Source), ObservedAt: now.UTC(), EvaluatedAt: x.At, State: "usable", Good: x.Good, Total: x.Total, Coverage: x.Coverage, Checks: []v1.QualityCheck{}, Limitations: []string{"Coverage and event counts do not establish statistical confidence or business correctness.", "Observation at one service does not establish end-to-end critical user journey success."}, Freshness: v1.Freshness{State: "missing", StaleAfterSeconds: 2 * c.StepSeconds}}
	q.CollectedSourceDigest = r.Collection.SourceDigest
	if !r.Collection.SourceFrom.IsZero() {
		from := r.Collection.SourceFrom
		q.SourceFrom = &from
	}
	if !r.Collection.SourceTo.IsZero() {
		to := r.Collection.SourceTo
		q.SourceTo = &to
	}
	add := func(code, state, message string) {
		q.Checks = append(q.Checks, v1.QualityCheck{Code: code, State: state, Message: message})
		if state == "failed" {
			q.State = "unavailable"
		} else if state == "warning" && q.State == "usable" {
			q.State = "limited"
		}
	}
	if s.CollectorFor(c) != nil && c.HasQueries() {
		q.Freshness.StaleAfterSeconds += int64(collectionInterval / time.Second)
	}
	if c.Quality != nil {
		q.MinimumEvents = c.Quality.MinEvents
		if c.Quality.MaxSampleAgeSeconds > 0 {
			q.Freshness.StaleAfterSeconds = c.Quality.MaxSampleAgeSeconds
		}
	}
	latest := func(at time.Time) {
		if !at.After(x.At) && (q.Freshness.LastSampleAt == nil || at.After(*q.Freshness.LastSampleAt)) {
			t := at
			q.Freshness.LastSampleAt = &t
		}
	}
	if c.ProviderWindow != nil {
		for _, o := range r.Windows {
			if o.WindowSeconds == c.WindowSeconds && o.Revision == model.ConfigRevision(c) {
				latest(o.Time)
			}
		}
	} else {
		for _, p := range r.Points {
			latest(p.Time)
		}
	}
	reference := now
	if historical {
		reference = x.At
		q.Limitations = append(q.Limitations, "Historical evidence is descriptive and cannot authorize a current operational decision.")
	}
	if q.Freshness.LastSampleAt != nil {
		age := max(int64(reference.Sub(*q.Freshness.LastSampleAt)/time.Second), 0)
		q.Freshness.AgeAtObservationSeconds = &age
		q.Freshness.State = "fresh"
		if age > q.Freshness.StaleAfterSeconds {
			q.Freshness.State = "stale"
		}
	}
	if q.Freshness.State == "fresh" {
		add("freshness", "satisfied", "The completed measurement cutoff is within the allowed age.")
	} else {
		add("freshness", "failed", "Measurement evidence is missing or older than the allowed age.")
	}
	if x.Coverage == 1 {
		add("coverage", "satisfied", "The evaluated window has complete reported measurement coverage.")
	} else {
		add("coverage", "warning", "The window is incomplete; observed attainment does not establish full-window compliance.")
	}
	if x.Total <= 0 || x.Reliability == nil {
		add("denominator", "failed", "No usable nonzero event population is available. Missing data and zero traffic are not success.")
	} else if x.Total < q.MinimumEvents {
		add("denominator", "warning", "Observed eligible events are below the configured quality minimum.")
	} else {
		add("denominator", "satisfied", "Observed eligible events meet the configured count requirement.")
	}
	if c.Measurement != nil {
		q.ObservationPoint = c.Measurement.ObservationPoint
	}
	add("observation_scope", "not_assessed", "Review the declared observation point, excluded traffic and retry semantics; instrumentation completeness is a human assertion.")
	if health, ok := s.measurementHealth(r); ok {
		if health.EvaluationBlocked {
			add("measurement_contract", "failed", "The saved measurement contract or source route no longer matches verified evidence.")
		} else if health.State != "compatible" {
			add("measurement_contract", "warning", "Measurement metadata verification is stale or inconclusive.")
		} else {
			add("measurement_contract", "satisfied", "Recent metadata matches this measurement revision and source route.")
		}
	} else if c.Measurement != nil || c.MeasurementBinding != nil {
		add("measurement_contract", "warning", "This structured measurement has no current revision-bound metadata verification.")
	} else {
		add("measurement_contract", "not_assessed", "Custom or imported counts have no structured metadata contract verification.")
	}
	if c.HasQueries() {
		lineage := r.Collection
		if lineage.SourceDigest == "" {
			add("source_lineage", "warning", "Retained samples have no collector source fingerprint. Collect or backfill the window to verify source lineage.")
		} else if lineage.SourceDigest != q.SourceDigest {
			add("source_lineage", "failed", "The source route or credential identity changed after these samples were collected.")
		} else if lineage.SourceFrom.IsZero() || lineage.SourceTo.IsZero() || x.From.Before(lineage.SourceFrom) || x.At.After(lineage.SourceTo) {
			add("source_lineage", "warning", "The full evaluated window is not covered by the current source collection lineage. Backfill the complete window.")
		} else {
			add("source_lineage", "satisfied", "The complete window is covered by collection evidence from the current source fingerprint.")
		}
	}
	if !historical && c.HasQueries() && s.CollectorFor(c) == nil {
		add("connection", "failed", "The configured measurement source is unavailable.")
	}
	if !historical && r.Collection.SLORevision == model.ConfigRevision(c) && r.Collection.State == "failed" {
		add("collection", "warning", "The most recent collection failed; retained samples do not prove current source availability.")
	}
	return q
}

// MeasurementQuality reads saved evidence by default. Cohort inspection is an
// explicit, bounded provider read and never persists samples or deploys rules.
func (s *Service) MeasurementQuality(ctx context.Context, id string, expected int64, cohorts bool) (v1.MeasurementQuality, error) {
	if expected < 0 {
		return v1.MeasurementQuality{}, model.Invalid("expectedRevision must be nonnegative")
	}
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return v1.MeasurementQuality{}, err
	}
	if expected > 0 && model.ConfigRevision(r.SLO) != expected {
		return v1.MeasurementQuality{}, model.ErrRevisionConflict
	}
	x, err := s.statusFromRecord(r, time.Time{})
	if err != nil {
		return v1.MeasurementQuality{}, err
	}
	q := s.qualityFromRecord(r, x, s.Now(), false)
	if !cohorts {
		return q, nil
	}
	report := v1.CohortReport{State: "not_configured", Message: "Configure quality.cohortLabels in the SLO to inspect subpopulations.", At: x.At, WindowSeconds: r.SLO.WindowSeconds, Items: []v1.CohortResult{}, Labels: []string{}}
	q.Cohorts = &report
	if r.SLO.Quality == nil || len(r.SLO.Quality.CohortLabels) == 0 {
		return q, nil
	}
	report.Labels = append([]string(nil), r.SLO.Quality.CohortLabels...)
	reader, ok := s.CollectorFor(r.SLO).(CohortSource)
	if !ok || r.SLO.ProviderWindow == nil {
		report.State = "unsupported"
		report.Message = "This provider or measurement mode cannot supply cohort counts."
		return q, nil
	}
	job, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	evidence, err := reader.ReadCohorts(job, r.SLO, report.Labels, x.At)
	if ctx.Err() != nil {
		return q, ctx.Err()
	}
	latest, readErr := s.Store.Read(ctx, id)
	if readErr != nil {
		return q, readErr
	}
	if model.ConfigRevision(latest.SLO) != q.Revision || s.SourceFingerprint(r.SLO.Source) != q.SourceDigest {
		return q, model.ErrRevisionConflict
	}
	report.State = "unavailable"
	report.Message = "The provider could not supply complete, valid cohort evidence."
	if err != nil {
		return q, nil
	}
	if len(evidence.Counts) > 100 {
		report.State = "limited"
		report.Message = "More than 100 cohorts matched. Narrow the population or use fewer cohort labels; no partial ranking is shown."
		return q, nil
	}
	if !evidence.At.Equal(x.At) || evidence.WindowSeconds != r.SLO.WindowSeconds || len(evidence.Counts) == 0 {
		return q, nil
	}
	items := []v1.CohortResult{}
	seen := map[string]bool{}
	total, good := 0.0, 0.0
	var totalCorrection, goodCorrection float64
	addCount := func(sum, correction *float64, value float64) {
		y := value - *correction
		next := *sum + y
		*correction = (next - *sum) - y
		*sum = next
	}
	complete := true
	for _, item := range evidence.Counts {
		if len(item.Labels) != len(report.Labels) || !model.Finite(item.Total) || item.Total < 0 || item.Total > 1e18 {
			return q, nil
		}
		labels := map[string]string{}
		for _, label := range report.Labels {
			value, found := item.Labels[label]
			if !found || len(value) > 200 {
				return q, nil
			}
			labels[label] = value
		}
		if err := model.ValidateLabels(labels); err != nil {
			return q, nil
		}
		key := model.DefinitionDigest(labels)
		if seen[key] {
			return q, nil
		}
		seen[key] = true
		result := v1.CohortResult{Labels: labels, Good: item.Good, Total: item.Total, State: "no_traffic"}
		addCount(&total, &totalCorrection, item.Total)
		if item.Good == nil {
			result.State = "missing_numerator"
			complete = false
		} else {
			if !model.Finite(*item.Good) || *item.Good < 0 || *item.Good > item.Total {
				return q, nil
			}
			addCount(&good, &goodCorrection, *item.Good)
			if item.Total > 0 {
				reliability := *item.Good / item.Total
				burn := (1 - reliability) / (1 - r.SLO.Target)
				result.Reliability = &reliability
				result.BurnRate = &burn
				result.State = "observed"
				if item.Total < q.MinimumEvents {
					result.State = "low_traffic"
				}
			}
		}
		items = append(items, result)
	}
	report.Items = items
	report.MatchesAggregate = complete && math.Abs(total-x.Total) <= 8*(math.Nextafter(math.Max(1, x.Total), math.Inf(1))-math.Max(1, x.Total)) && math.Abs(good-x.Good) <= 8*(math.Nextafter(math.Max(1, x.Good), math.Inf(1))-math.Max(1, x.Good))
	report.State = "available"
	report.Message = "Observed cohort estimates at the same cutoff. Missing numerators remain unknown; cohort coverage and statistical confidence are not independently verified."
	sort.Slice(report.Items, func(i, j int) bool {
		a, b := report.Items[i], report.Items[j]
		if a.Reliability == nil {
			return b.Reliability != nil
		}
		if b.Reliability == nil {
			return false
		}
		return *a.Reliability < *b.Reliability
	})
	return q, nil
}
