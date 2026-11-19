package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/slotemplates"
	"github.com/ishantanu/nines/internal/telemetryprovider"
)

func validateIntentCandidateInput(in v1.IntentCandidateInput) (*float64, error) {
	if !intentScopeText(in.Service) || !intentScopeText(in.Project) || !validResourceID(in.Source) || len(in.Population) == 0 || model.ValidateLabels(in.Population) != nil || in.Limit < 0 || in.Limit > 3 || len(in.Metrics) > 3 || len(in.Category) > 63 || len(in.Prefix) > 100 || (in.Prefix != "" && !model.MetricName.MatchString(in.Prefix)) || (in.Outcome != "" && in.Outcome != "availability" && in.Outcome != "latency") {
		return nil, model.Invalid("provide exact service/project/source, 1–16 equality population labels, a literal metric prefix and limit 1–3")
	}
	for _, metric := range in.Metrics {
		if !model.MetricName.MatchString(metric) || !strings.HasPrefix(metric, in.Prefix) {
			return nil, model.Invalid("explicit metrics must be literal names matching the prefix")
		}
	}
	if in.Threshold == "" {
		return nil, nil
	}
	duration, err := time.ParseDuration(in.Threshold)
	if err != nil || duration <= 0 || duration > 24*time.Hour || len(in.Threshold) > 32 || in.Outcome != "latency" {
		return nil, model.Invalid("latency threshold requires explicit units, such as 30ms, and must be positive and at most 24h")
	}
	seconds := duration.Seconds()
	return &seconds, nil
}

// DiscoverIntentCandidates reuses the optional provider interfaces and a pinned,
// server-installed recipe catalog. It never generates queries or confirms semantics.
func (s *Service) DiscoverIntentCandidates(ctx context.Context, in v1.IntentCandidateInput, templates *slotemplates.Registry) (v1.IntentCandidateDiscovery, error) {
	threshold, err := validateIntentCandidateInput(in)
	if err != nil {
		return v1.IntentCandidateDiscovery{}, err
	}
	in.Population = maps.Clone(in.Population)
	in.Metrics = slices.Clone(in.Metrics)
	if in.Limit == 0 {
		in.Limit = 3
	}
	if in.Source != "primary" {
		_, found := s.Sources[in.Source]
		if !found && s.DynamicSource != nil {
			_, found = s.DynamicSource(in.Source)
		}
		if !found {
			return v1.IntentCandidateDiscovery{}, model.ErrNotFound
		}
	}
	catalog, err := templates.List(in.Category)
	if err != nil {
		return v1.IntentCandidateDiscovery{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	collector := s.CollectorFor(model.SLO{Source: in.Source})
	namesProvider, namesSupported := collector.(telemetryprovider.MetricNamesSource)
	seriesProvider, seriesSupported := collector.(telemetryprovider.MetricDiscoverySource)
	fingerprint := s.SourceFingerprint(in.Source)
	now := s.Now().UTC().Truncate(time.Second)
	out := v1.IntentCandidateDiscovery{APIVersion: v1.APIVersion, Tenant: s.TenantID, ObservedAt: now, TextTrust: resourceTextTrust, Scope: in, SourceDigest: s.IntentSourceDigest(in.Source), CatalogDigest: catalog.Digest, State: "unsupported", MetricNameDiscovery: namesSupported, SeriesDiscovery: seriesSupported, MaxAdapterCalls: 1 + in.Limit, Names: []string{}, Candidates: []v1.IntentMeasurementCandidate{}, Questions: []string{}, Warnings: []string{"discovery_is_not_measurement_confirmation_or_approval", "series_presence_is_not_fresh_traffic_or_measurement_quality", "service_and_project_do_not_infer_population_filters", "recipe_matches_are_unverified_suggestions", "adapter_calls_are_not_underlying_http_request_counts"}, HumanConfirmationRequired: true}
	if !seriesSupported || (len(in.Metrics) == 0 && !namesSupported) {
		out.Questions = append(out.Questions, "Use an adapter with metric and series discovery, or supply a reviewed measurement through the existing definition workflow.")
		return out, nil
	}
	names := slices.Clone(in.Metrics)
	if len(names) == 0 {
		out.AdapterCalls++
		call, cancel := context.WithTimeout(ctx, 4*time.Second)
		names, out.Truncated, err = namesProvider.MetricNames(call, in.Prefix, now)
		cancel()
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if s.SourceFingerprint(in.Source) != fingerprint {
			return v1.IntentCandidateDiscovery{}, model.ErrRevisionConflict
		}
		if err != nil {
			out.State = "provider_error"
			out.Questions = append(out.Questions, "Metric discovery failed. Check source permissions and retry with a narrower prefix.")
			return out, nil
		}
	}
	if len(names) > 101 {
		out.State = "provider_error"
		out.Warnings = append(out.Warnings, "adapter_metric_names_exceed_bounds")
		return out, nil
	}
	for _, name := range names {
		if !model.MetricName.MatchString(name) || !strings.HasPrefix(name, in.Prefix) {
			out.State = "provider_error"
			out.Warnings = append(out.Warnings, "adapter_returned_invalid_metric_names")
			return out, nil
		}
	}
	names = slices.Clone(names)
	sort.Strings(names)
	names = slices.Compact(names)
	out.Names = append(out.Names, names...)
	if len(out.Names) > 50 {
		out.Names = out.Names[:50]
		out.Truncated = true
	}
	// Prefer exact catalog exemplars, while keeping all names as untrusted evidence.
	// A name match never establishes the exemplar's success or population semantics.
	sort.SliceStable(names, func(i, j int) bool {
		return candidateNameRank(names[i], catalog.Templates) > candidateNameRank(names[j], catalog.Templates)
	})
	selected := names
	if len(selected) > in.Limit {
		selected = selected[:in.Limit]
		out.Truncated = true
	}
	out.State = "no_matching_metrics"
	failures, seen := 0, 0
	for _, metric := range selected {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if s.SourceFingerprint(in.Source) != fingerprint {
			return v1.IntentCandidateDiscovery{}, model.ErrRevisionConflict
		}
		out.AdapterCalls++
		call, cancel := context.WithTimeout(ctx, 4*time.Second)
		evidence, readErr := seriesProvider.Discover(call, metric, maps.Clone(in.Population), now)
		cancel()
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if s.SourceFingerprint(in.Source) != fingerprint {
			return v1.IntentCandidateDiscovery{}, model.ErrRevisionConflict
		}
		candidate := v1.IntentMeasurementCandidate{RequestedMetric: metric, Metric: metric, State: "provider_error", MetricType: "unknown", Unit: "unknown", LabelNames: []string{}, Recipes: []v1.IntentRecipeReference{}, Questions: []string{}, Warnings: []string{}, ThresholdSeconds: threshold}
		if readErr != nil {
			failures++
			candidate.Questions = append(candidate.Questions, "Series inspection failed. Check query permissions and retry this metric.")
		} else if candidateErr := readCandidateEvidence(&candidate, evidence, in, now, catalog.Templates); candidateErr != nil {
			failures++
			candidate.Questions = append(candidate.Questions, "The adapter returned incomplete or incompatible discovery evidence. Narrow the population and retry.")
			candidate.Warnings = append(candidate.Warnings, "invalid_discovery_receipt")
		} else if candidate.ObservedSeries > 0 {
			seen++
		}
		candidate.EvidenceDigest = model.DefinitionDigest(struct {
			Source, Catalog string
			Candidate       v1.IntentMeasurementCandidate
		}{out.SourceDigest, out.CatalogDigest, candidate})
		candidate.ID = "candidate-" + candidate.EvidenceDigest[:16]
		out.Truncated = out.Truncated || candidate.Truncated
		out.Candidates = append(out.Candidates, candidate)
	}
	switch {
	case failures > 0 && failures == len(out.Candidates):
		out.State = "provider_error"
	case failures > 0 || out.Truncated:
		out.State = "partial"
	case seen > 0:
		out.State = "needs_input"
	}
	if seen == 0 {
		out.Questions = append(out.Questions, "No matching series were established in this bounded lookup. Confirm instrumentation, labels, permissions and retention; this does not prove telemetry is absent.")
	}
	if out.Truncated {
		out.Questions = append(out.Questions, "Discovery is partial. Narrow the prefix or population, or inspect exact returned metric names.")
	}
	out.Questions = append(out.Questions, "Confirm the eligible population, success criteria, observation point and retry treatment before defining an SLO.")
	encoded, err := json.Marshal(out)
	if err != nil {
		return v1.IntentCandidateDiscovery{}, err
	}
	if len(encoded) > 128<<10 {
		return v1.IntentCandidateDiscovery{}, model.Invalid("candidate response exceeds 128 KiB; narrow the request")
	}
	return out, nil
}
func candidateNameRank(metric string, templates []v1.SLOTemplate) int {
	for _, template := range templates {
		if template.Manifest.Spec.Measurement == nil {
			continue
		}
		base := template.Manifest.Spec.Measurement.Metric
		if metric == base || metric == base+"_bucket" || metric == base+"_count" {
			return 1
		}
	}
	return 0
}
func readCandidateEvidence(candidate *v1.IntentMeasurementCandidate, e v1.TelemetryDiscovery, in v1.IntentCandidateInput, now time.Time, templates []v1.SLOTemplate) error {
	// Commit validated evidence together. A rejected receipt cannot retain partial
	// unit, bucket or population assertions in a provider-error candidate.
	validated := *candidate
	if err := inspectCandidateEvidence(&validated, e, in, now, templates); err != nil {
		return err
	}
	*candidate = validated
	return nil
}
func inspectCandidateEvidence(candidate *v1.IntentMeasurementCandidate, e v1.TelemetryDiscovery, in v1.IntentCandidateInput, now time.Time, templates []v1.SLOTemplate) error {
	if len(e.Metadata) > 10 || len(e.Series) > 100 || e.From.IsZero() || e.To.IsZero() || !e.To.After(e.From) || e.To.After(now) || e.From.Before(now.Add(-24*time.Hour)) {
		return errors.New("invalid discovery bounds")
	}
	types, units := map[string]bool{}, map[string]bool{}
	for _, metadata := range e.Metadata {
		switch metadata.Type {
		case "counter", "histogram", "gauge", "summary", "untyped":
			types[metadata.Type] = true
		default:
			types["unknown"] = true
		}
		switch metadata.Unit {
		case "s", "seconds":
			units["seconds"] = true
		case "ms", "milliseconds":
			units["milliseconds"] = true
		default:
			units["unknown"] = true
		}
	}
	if len(types) == 1 {
		for metricType := range types {
			candidate.MetricType = metricType
		}
	}
	if len(units) == 1 {
		for unit := range units {
			candidate.Unit = unit
		}
	}
	canonical := candidate.RequestedMetric
	if candidate.MetricType == "histogram" {
		for _, suffix := range []string{"_bucket", "_count", "_sum"} {
			if strings.HasSuffix(canonical, suffix) {
				canonical = strings.TrimSuffix(canonical, suffix)
				break
			}
		}
	}
	if e.Metric != candidate.RequestedMetric && e.Metric != canonical {
		return errors.New("unmatched metric")
	}
	candidate.Metric = e.Metric
	labels := map[string]bool{}
	bucket, count := false, false
	for _, series := range e.Series {
		if len(series) > 40 {
			return errors.New("excess labels")
		}
		for k, v := range series {
			if !model.LabelName.MatchString(k) || len(v) > 2000 {
				return errors.New("invalid labels")
			}
			if k != "__name__" && k != "le" {
				labels[k] = true
			}
		}
		for k, v := range in.Population {
			if series[k] != v {
				return errors.New("population mismatch")
			}
		}
		name := series["__name__"]
		histogramSeries := candidate.MetricType == "histogram" && (name == candidate.Metric+"_bucket" || name == candidate.Metric+"_count")
		if name != candidate.Metric && !histogramSeries {
			return errors.New("series metric mismatch")
		}
		if name == candidate.Metric+"_bucket" {
			bucket = true
			if candidate.ThresholdSeconds != nil {
				value, err := strconv.ParseFloat(series["le"], 64)
				candidate.MatchingBoundaryObserved = candidate.MatchingBoundaryObserved || (err == nil && value == *candidate.ThresholdSeconds)
			}
		}
		count = count || name == candidate.Metric+"_count"
	}
	for label := range labels {
		candidate.LabelNames = append(candidate.LabelNames, label)
	}
	sort.Strings(candidate.LabelNames)
	candidate.From = e.From
	candidate.To = e.To
	candidate.ObservedSeries = len(e.Series)
	candidate.Truncated = e.Truncated
	candidate.State = "needs_input"
	if len(e.Series) == 0 {
		candidate.State = "no_matching_series"
		candidate.Questions = append(candidate.Questions, "Confirm that this metric exists for the exact requested population and lookup interval.")
		return nil
	}
	measurementType := ""
	switch candidate.MetricType {
	case "counter":
		measurementType = "availability"
		candidate.Questions = append(candidate.Questions, "Define good events and confirm whether the counter counts logical outcomes or individual attempts, including retries.")
	case "histogram":
		measurementType = "latency"
		if !bucket || !count {
			candidate.Questions = append(candidate.Questions, "Classic latency measurement needs both bucket and count series for the same population.")
		}
		if candidate.Unit != "seconds" {
			candidate.Questions = append(candidate.Questions, "Confirm the histogram unit is seconds before using a seconds-based threshold.")
		}
		if candidate.ThresholdSeconds == nil {
			candidate.Questions = append(candidate.Questions, "Supply an explicit latency threshold with units, such as 30ms.")
		} else if !candidate.MatchingBoundaryObserved {
			candidate.Questions = append(candidate.Questions, "The exact threshold boundary was not observed. Review instrumentation; do not substitute a percentile or the next bucket.")
		}
	default:
		candidate.State = "unsupported_measurement"
		candidate.Questions = append(candidate.Questions, "Use eligible event counters or a compatible duration histogram. Gauges and summary percentiles do not establish a good-events ratio.")
	}
	if in.Outcome != "" && measurementType != "" && in.Outcome != measurementType {
		candidate.State = "incompatible_outcome"
		candidate.Questions = append(candidate.Questions, "This metric type does not match the requested outcome. Review a different metric.")
	}
	exact := false
	for _, template := range templates {
		if template.Manifest.Spec.Measurement != nil && template.Manifest.Spec.Measurement.Type == measurementType && template.Manifest.Spec.Measurement.Metric == candidate.Metric {
			exact = true
		}
	}
	for _, template := range templates {
		m := template.Manifest.Spec.Measurement
		if m == nil || m.Type != measurementType || (exact && m.Metric != candidate.Metric) {
			continue
		}
		basis := "measurement_type_only"
		if exact {
			basis = "example_metric_name_only"
		}
		candidate.Recipes = append(candidate.Recipes, v1.IntentRecipeReference{ID: template.ID, Revision: template.Revision, Digest: template.Digest, Category: template.Category, SLI: template.SLI, Match: basis, Extension: template.Extension})
		if strings.Contains(template.SLI, "evaluation") {
			candidate.Questions = append(candidate.Questions, "Pin the rubric, evaluator version and sampling cohort. Keep failed and unknown verdicts in the denominator and check evaluation completion coverage separately.")
		}
		if template.SLI == "task-success" {
			candidate.Questions = append(candidate.Questions, "Confirm one terminal outcome per logical task; tool-call success does not establish task completion.")
		}
		if len(candidate.Recipes) == 4 {
			candidate.Warnings = append(candidate.Warnings, "recipe_suggestions_bounded_review_catalog_for_more")
			break
		}
	}
	candidate.Questions = append(candidate.Questions, "Confirm the observation point and excluded traffic; metric names and recipe matches do not prove business semantics.")
	candidate.Warnings = append(candidate.Warnings, "provider_help_and_unrequested_label_values_are_not_forwarded")
	return nil
}
