package v1beta1

import (
	"fmt"
	"math"
	"time"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

// Defaults for omitted settings. Plan output reports each one applied.
const (
	DefaultWindow         = 28 * 24 * time.Hour
	DefaultStep           = time.Minute
	DefaultScrapeInterval = 15 * time.Second
)

// standardPolicies are the multiwindow, multi-burn-rate alerts from the SRE
// workbook: page when 2% of the budget burns in an hour or 5% in six hours,
// and open a ticket when 10% burns in three days.
var standardPolicies = []v1alpha1.BurnRatePolicy{
	{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1},
	{Name: "sustained-burn", Severity: "critical", ShortWindowSeconds: 1800, LongWindowSeconds: 21600, BurnRate: 6, MinCoverage: 1},
	{Name: "slow-burn", Severity: "warning", ShortWindowSeconds: 21600, LongWindowSeconds: 259200, BurnRate: 1, MinCoverage: 1},
}

// DefaultPolicies returns the standard alerts that fit an SLO, with a note for
// each one left out and why.
func DefaultPolicies(target float64, windowSeconds, stepSeconds int64) ([]v1alpha1.BurnRatePolicy, []string) {
	out := []v1alpha1.BurnRatePolicy{}
	notes := []string{}
	for _, p := range standardPolicies {
		switch {
		case p.LongWindowSeconds > windowSeconds:
			notes = append(notes, fmt.Sprintf("left out the default %s alert: its %s window is longer than the %s SLO window", p.Name, D(time.Duration(p.LongWindowSeconds)*time.Second), D(time.Duration(windowSeconds)*time.Second)))
		case stepSeconds > 0 && (p.ShortWindowSeconds%stepSeconds != 0 || p.LongWindowSeconds%stepSeconds != 0):
			notes = append(notes, fmt.Sprintf("left out the default %s alert: its windows are not multiples of the %s step", p.Name, D(time.Duration(stepSeconds)*time.Second)))
		case target < 1 && p.BurnRate > 1/(1-target):
			notes = append(notes, fmt.Sprintf("left out the default %s alert: a %gx burn rate is unreachable with a %g target", p.Name, p.BurnRate, target))
		default:
			out = append(out, p)
		}
	}
	return out, notes
}

func seconds(d *Duration, def time.Duration, field string) (int64, error) {
	if d == nil {
		return int64(def / time.Second), nil
	}
	return d.WholeSeconds(field)
}

func dur(seconds int64) *Duration {
	d := D(time.Duration(seconds) * time.Second)
	return &d
}

// ToHub converts an SLO to the stored v1alpha1 form, applying defaults. The
// notes describe every default applied, for plan output.
func (s ServiceLevelObjective) ToHub() (v1alpha1.ServiceLevelObjective, []string, error) {
	var notes []string
	if s.APIVersion != APIVersion || s.Kind != SLOKind {
		return v1alpha1.ServiceLevelObjective{}, nil, fmt.Errorf("expected apiVersion %s and kind %s", APIVersion, SLOKind)
	}
	in := s.Spec
	if in.Window == nil {
		notes = append(notes, "window defaults to "+D(DefaultWindow).String())
	}
	window, err := seconds(in.Window, DefaultWindow, "window")
	if err != nil {
		return v1alpha1.ServiceLevelObjective{}, nil, err
	}
	step, err := seconds(in.Step, DefaultStep, "step")
	if err != nil {
		return v1alpha1.ServiceLevelObjective{}, nil, err
	}
	out := v1alpha1.SLOSpec{
		DisplayName: in.DisplayName, Service: in.Service, Target: in.Target,
		WindowSeconds: window, StepSeconds: step, Source: in.Connection,
		Labels: in.Labels, Lifecycle: in.Lifecycle, ErrorBudgetPolicy: in.ErrorBudgetPolicy,
		Customers: in.Customers, MeasurementBinding: in.Binding,
	}
	sources := 0
	if in.Measurement != nil {
		sources++
		m := in.Measurement
		scrape, err := seconds(m.ScrapeInterval, DefaultScrapeInterval, "measurement.scrapeInterval")
		if err != nil {
			return v1alpha1.ServiceLevelObjective{}, nil, err
		}
		labels := m.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		out.Measurement = &v1alpha1.MeasurementSpec{Type: m.Type, Metric: m.Metric, EventMetric: m.EventMetric, Labels: labels, SuccessLabels: m.SuccessLabels, CountFrom: m.CountFrom, ScrapeIntervalSeconds: scrape, ObservationPoint: m.ObservationPoint}
		if m.Threshold != nil {
			t := m.Threshold.Seconds()
			out.Measurement.ThresholdSeconds = &t
		}
	}
	if q := in.Queries; q != nil {
		sources++
		if (q.Good != "" || q.Total != "") == (q.Window != nil) {
			return v1alpha1.ServiceLevelObjective{}, nil, fmt.Errorf("queries need either good and total, or window selectors, not both")
		}
		out.GoodQuery, out.TotalQuery, out.QueryLanguage = q.Good, q.Total, q.Language
		if w := q.Window; w != nil {
			scrape, err := seconds(w.ScrapeInterval, DefaultScrapeInterval, "queries.window.scrapeInterval")
			if err != nil {
				return v1alpha1.ServiceLevelObjective{}, nil, err
			}
			out.ProviderWindow = &v1alpha1.ProviderWindow{GoodSelector: w.Good, TotalSelector: w.Total, BadSelector: w.Bad, ScrapeIntervalSeconds: scrape}
		}
	}
	if in.Binding != nil {
		sources++
	}
	if sources > 1 {
		return v1alpha1.ServiceLevelObjective{}, nil, fmt.Errorf("an SLO is measured by at most one of measurement, queries or binding; found %d", sources)
	}
	if sources == 0 {
		notes = append(notes, "no measurement, queries or binding: the SLO is measured from samples pushed to the API")
	}
	if iv := in.Intervals; iv != nil {
		if iv.MinGoodEvents > 0 && (iv.ErrorRate > 0 || iv.MinEvents > 0) {
			return v1alpha1.ServiceLevelObjective{}, nil, fmt.Errorf("intervals take errorRate (good-minute SLO) or minGoodEvents (throughput SLO), not both")
		}
		out.BadInterval = &v1alpha1.BadIntervalPolicy{ErrorRate: iv.ErrorRate, MinEvents: iv.MinEvents, MinGoodEvents: iv.MinGoodEvents}
	}
	if q := in.Quality; q != nil {
		out.Quality = &v1alpha1.MeasurementQualityPolicy{MinEvents: q.MinEvents, CohortLabels: q.CohortLabels}
		if q.MaxSampleAge != nil {
			if out.Quality.MaxSampleAgeSeconds, err = q.MaxSampleAge.WholeSeconds("quality.maxSampleAge"); err != nil {
				return v1alpha1.ServiceLevelObjective{}, nil, err
			}
		}
	}
	if in.Policies == nil {
		policies, left := DefaultPolicies(in.Target, window, step)
		out.Policies = policies
		names := ""
		for i, p := range policies {
			if i > 0 {
				names += ", "
			}
			names += p.Name
		}
		if len(policies) > 0 {
			notes = append(notes, "policies default to the standard burn-rate alerts: "+names)
		}
		notes = append(notes, left...)
	} else {
		for i, p := range *in.Policies {
			hp := v1alpha1.BurnRatePolicy{Name: p.Name, Severity: p.Severity, BurnRate: p.BurnRate, MinCoverage: 1, MinEvents: p.MinEvents, Confidence: p.Confidence}
			if hp.ShortWindowSeconds, err = p.ShortWindow.WholeSeconds(fmt.Sprintf("policies[%d].shortWindow", i)); err != nil {
				return v1alpha1.ServiceLevelObjective{}, nil, err
			}
			if hp.LongWindowSeconds, err = p.LongWindow.WholeSeconds(fmt.Sprintf("policies[%d].longWindow", i)); err != nil {
				return v1alpha1.ServiceLevelObjective{}, nil, err
			}
			if p.For != nil {
				if hp.ForSeconds, err = p.For.WholeSeconds(fmt.Sprintf("policies[%d].for", i)); err != nil {
					return v1alpha1.ServiceLevelObjective{}, nil, err
				}
			}
			if p.MinCoverage != nil {
				hp.MinCoverage = *p.MinCoverage
			}
			out.Policies = append(out.Policies, hp)
		}
		if len(out.Policies) == 0 {
			notes = append(notes, "alerting is off: policies is an empty list")
		}
	}
	return v1alpha1.NewSLO(s.Metadata.Name, s.Metadata.Project, out), notes, nil
}

// FromHub converts a stored v1alpha1 SLO to v1beta1 for export. An SLO that
// had no alert policies keeps none (an explicit empty list), so converting a
// definition never starts alerting. Server-owned provenance (spec.intent) and
// window selectors derived from a measurement are not part of the result.
func FromHub(h v1alpha1.ServiceLevelObjective) ServiceLevelObjective {
	in := h.Spec
	out := SLOSpec{DisplayName: in.DisplayName, Service: in.Service, Target: in.Target, Window: dur(in.WindowSeconds), Step: dur(in.StepSeconds), Connection: in.Source, Labels: in.Labels, Lifecycle: in.Lifecycle, ErrorBudgetPolicy: in.ErrorBudgetPolicy, Customers: in.Customers}
	if out.Connection == "primary" {
		out.Connection = ""
	}
	switch {
	case in.Measurement != nil:
		m := in.Measurement
		out.Measurement = &MeasurementSpec{Type: m.Type, Metric: m.Metric, EventMetric: m.EventMetric, Labels: m.Labels, SuccessLabels: m.SuccessLabels, CountFrom: m.CountFrom, ObservationPoint: m.ObservationPoint}
		if m.ThresholdSeconds != nil {
			t := D(time.Duration(math.Round(*m.ThresholdSeconds * float64(time.Second))))
			out.Measurement.Threshold = &t
		}
		if m.ScrapeIntervalSeconds != int64(DefaultScrapeInterval/time.Second) {
			out.Measurement.ScrapeInterval = dur(m.ScrapeIntervalSeconds)
		}
	case in.MeasurementBinding != nil:
		out.Binding = in.MeasurementBinding
	case in.GoodQuery == "" && in.TotalQuery == "" && in.ProviderWindow == nil:
		// Measured from samples pushed to the API.
	default:
		q := &Queries{Good: in.GoodQuery, Total: in.TotalQuery, Language: in.QueryLanguage}
		if w := in.ProviderWindow; w != nil {
			q.Window = &WindowSelectors{Good: w.GoodSelector, Total: w.TotalSelector, Bad: w.BadSelector}
			if w.ScrapeIntervalSeconds != int64(DefaultScrapeInterval/time.Second) {
				q.Window.ScrapeInterval = dur(w.ScrapeIntervalSeconds)
			}
		}
		out.Queries = q
	}
	if b := in.BadInterval; b != nil {
		out.Intervals = &Intervals{ErrorRate: b.ErrorRate, MinEvents: b.MinEvents, MinGoodEvents: b.MinGoodEvents}
	}
	if q := in.Quality; q != nil {
		out.Quality = &QualityPolicy{MinEvents: q.MinEvents, CohortLabels: q.CohortLabels}
		if q.MaxSampleAgeSeconds > 0 {
			out.Quality.MaxSampleAge = dur(q.MaxSampleAgeSeconds)
		}
	}
	hubPolicies := in.Policies
	if len(hubPolicies) == 0 && in.Policy != nil {
		hubPolicies = []v1alpha1.BurnRatePolicy{*in.Policy}
	}
	policies := []BurnRatePolicy{}
	for _, p := range hubPolicies {
		bp := BurnRatePolicy{Name: p.Name, Severity: p.Severity, ShortWindow: D(time.Duration(p.ShortWindowSeconds) * time.Second), LongWindow: D(time.Duration(p.LongWindowSeconds) * time.Second), BurnRate: p.BurnRate, MinEvents: p.MinEvents, Confidence: p.Confidence}
		if p.ForSeconds > 0 {
			bp.For = dur(p.ForSeconds)
		}
		if p.MinCoverage != 1 {
			c := p.MinCoverage
			bp.MinCoverage = &c
		}
		if bp.Name == "" {
			bp.Name = "burn"
		}
		if bp.Severity == "" {
			bp.Severity = "critical"
		}
		policies = append(policies, bp)
	}
	out.Policies = &policies
	return ServiceLevelObjective{APIVersion: APIVersion, Kind: SLOKind, Metadata: h.Metadata, Spec: out}
}
