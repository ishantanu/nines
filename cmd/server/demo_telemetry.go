package main

import (
	"context"
	"math"
	"strconv"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
)

// demoSource names the connection the typed demo SLOs read from. It is an
// in-process generator of synthetic counts: the demo never contacts Prometheus,
// Mimir or any vendor, and the default connection stays empty.
const demoSource = "demo-telemetry"

// demoTelemetry answers Nines' ordinary collection calls with deterministic
// synthetic counts, so typed demo SLOs exercise the real interval and
// provider-window evaluation paths instead of injected results.
type demoTelemetry struct{}

func (demoTelemetry) SourceFingerprint() string { return "demo-synthetic-telemetry" }

// demoRates returns the eligible events per minute and the share of them that
// were bad for the minute ending at t. Each profile is a function of time only,
// so repeated collection of the same range returns the same counts.
func demoRates(id string, t time.Time) (perMinute, bad float64) {
	hours := float64(t.Unix()) / 3600
	wave := math.Sin(2 * math.Pi * hours / 24) // one cycle a day
	switch id {
	case "search-latency":
		// Mostly fast, with a slow patch for a few hours each afternoon.
		slow := math.Max(0, math.Sin(2*math.Pi*hours/24))
		return 4000 + 1500*wave, .004 + .012*math.Pow(slow, 6)
	case "search-index-freshness":
		// A healthy indexer that lags behind the nightly catalog import.
		bad = .003
		if minute := t.UTC().Hour()*60 + t.UTC().Minute(); minute >= 6*60 && minute < 6*60+45 {
			bad = .09
		}
		return 600 + 100*wave, bad
	case "mobile-sessions":
		return 900 + 400*wave, .0035
	case "order-pipeline-throughput":
		// Received orders per minute; processed is derived in intervalCounts.
		return 900 + 250*wave, 0
	}
	return 1000, .001
}

// intervalCounts returns good and total for one step interval ending at t.
func intervalCounts(s model.SLO, t time.Time) (good, total float64) {
	per, bad := demoRates(s.ID, t)
	minutes := float64(s.StepSeconds) / 60
	total = math.Round(per * minutes)
	if s.BadInterval != nil && s.BadInterval.MinGoodEvents > 0 {
		// Throughput: the pipeline processes nearly everything it receives,
		// except a three-minute stall every eight hours.
		good = math.Round(total * .995)
		if t.Unix()/60%480 < 3 {
			good = math.Round(total * .2)
		}
		return good, total
	}
	return math.Round(total * (1 - bad)), total
}

func (demoTelemetry) Collect(_ context.Context, s model.SLO, from, to time.Time) ([]model.Point, error) {
	if s.ProviderWindow != nil {
		return nil, model.Invalid("use CollectWindows for overlapping provider observations")
	}
	step := time.Duration(s.StepSeconds) * time.Second
	out := []model.Point{}
	for t := from.Add(step); !t.After(to); t = t.Add(step) {
		good, total := intervalCounts(s, t)
		out = append(out, model.Point{Time: t, Good: good, Total: total})
	}
	return out, nil
}

// CollectWindows returns one complete observation per window and step, summed
// from the same per-interval counts, as a provider's window query would.
func (demoTelemetry) CollectWindows(_ context.Context, s model.SLO, from, to time.Time) ([]model.WindowObservation, error) {
	step := time.Duration(s.StepSeconds) * time.Second
	windows := s.Windows()
	longest := int64(0)
	for _, w := range windows {
		longest = max(longest, w)
	}
	// Prefix sums over every interval any requested window can reach.
	start := from.Add(-time.Duration(longest) * time.Second)
	goodSum, totalSum := []float64{0}, []float64{0}
	for t := start.Add(step); !t.After(to); t = t.Add(step) {
		good, total := intervalCounts(s, t)
		goodSum = append(goodSum, goodSum[len(goodSum)-1]+good)
		totalSum = append(totalSum, totalSum[len(totalSum)-1]+total)
	}
	index := func(t time.Time) int { return int(t.Sub(start) / step) }
	revision := model.ConfigRevision(s)
	out := []model.WindowObservation{}
	for t := from.Add(step); !t.After(to); t = t.Add(step) {
		end := index(t)
		for _, w := range windows {
			begin := end - int(w/s.StepSeconds)
			good, total := goodSum[end]-goodSum[begin], totalSum[end]-totalSum[begin]
			out = append(out, model.WindowObservation{Time: t, WindowSeconds: w, Revision: revision, Good: good, Total: total, Coverage: 1})
		}
	}
	return out, nil
}

// Discover describes the synthetic metrics the way a Prometheus source would,
// so the measurement contract check verifies the typed demo SLOs instead of
// reporting their telemetry as unavailable. Unknown metrics have no series.
func (demoTelemetry) Discover(_ context.Context, metric string, labels map[string]string, at time.Time) (v1.TelemetryDiscovery, error) {
	out := v1.TelemetryDiscovery{Metric: metric, From: at.Add(-time.Hour), To: at, Metadata: []v1.TelemetryMetadata{}, Series: []map[string]string{}}
	series := func(name string, extra map[string]string) map[string]string {
		s := map[string]string{"__name__": name}
		for k, v := range labels {
			s[k] = v
		}
		for k, v := range extra {
			s[k] = v
		}
		return s
	}
	for _, s := range demoTypedSLOs() {
		m := s.Measurement
		if m == nil {
			continue
		}
		switch {
		case m.Type == "sessions" && (metric == m.Metric || metric == m.EventMetric):
			out.Metadata = append(out.Metadata, v1.TelemetryMetadata{Type: "counter", Help: "Synthetic demo counter."})
			out.Series = append(out.Series, series(metric, nil))
		case metric == m.Metric && m.ThresholdSeconds != nil:
			out.Metadata = append(out.Metadata, v1.TelemetryMetadata{Type: "histogram", Help: "Synthetic demo histogram.", Unit: "seconds"})
			boundary := strconv.FormatFloat(*m.ThresholdSeconds, 'g', -1, 64)
			out.Series = append(out.Series, series(metric+"_count", nil), series(metric+"_bucket", map[string]string{"le": boundary}), series(metric+"_bucket", map[string]string{"le": "+Inf"}))
		}
	}
	return out, nil
}

// demoTypedSLOs shows the SLO types beyond plain event ratios. Each one is
// collected from demoSource through Nines' normal collection path.
func demoTypedSLOs() []model.SLO {
	policies := []model.Policy{{Name: "fast-burn", Severity: "critical", ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}, {Name: "sustained-burn", Severity: "warning", ShortWindowSeconds: 1800, LongWindowSeconds: 21600, BurnRate: 6, MinCoverage: 1}}
	threshold := func(v float64) *float64 { return &v }
	slo := func(id, service, name string, target float64, labels map[string]string) model.SLO {
		return model.SLO{ID: id, Project: "demo", Service: service, Name: name, Target: target, WindowSeconds: 86400, StepSeconds: 60, Source: demoSource, Labels: labels, Policies: policies}
	}
	latency := slo("search-latency", "search", "Search latency · demo", .99, map[string]string{"team": "discovery", "product": "storefront"})
	latency.Measurement = &v1.MeasurementSpec{Type: "latency", Metric: "http_server_request_duration_seconds", Labels: map[string]string{"service": "search"}, ThresholdSeconds: threshold(.3), ScrapeIntervalSeconds: 15, ObservationPoint: "Search API handler, from request received to response written."}
	freshness := slo("search-index-freshness", "indexer", "Search index freshness · demo", .99, map[string]string{"team": "discovery", "product": "storefront"})
	freshness.Measurement = &v1.MeasurementSpec{Type: "freshness", Metric: "index_record_age_seconds", Labels: map[string]string{"pipeline": "search-index"}, ThresholdSeconds: threshold(300), ScrapeIntervalSeconds: 15, ObservationPoint: "Age of each product record when search serves it, from its last catalog update."}
	sessions := slo("mobile-sessions", "mobile-app", "Crash-free app sessions · demo", .995, map[string]string{"team": "mobile", "product": "storefront"})
	sessions.Measurement = &v1.MeasurementSpec{Type: "sessions", Metric: "app_sessions_total", EventMetric: "app_sessions_crashed_total", Labels: map[string]string{"app": "ios"}, ScrapeIntervalSeconds: 15, ObservationPoint: "One increment per app session; the crash counter increments once per crashed session."}
	throughput := slo("order-pipeline-throughput", "workers", "Order pipeline throughput · demo", .99, map[string]string{"team": "platform", "product": "fulfillment"})
	throughput.GoodQuery = `sum(increase(order_pipeline_processed_total[1m]))`
	throughput.TotalQuery = `sum(increase(order_pipeline_received_total[1m]))`
	throughput.BadInterval = &v1.BadIntervalPolicy{MinGoodEvents: 500}
	return []model.SLO{latency, freshness, sessions, throughput}
}

// registerDemoSource adds the synthetic connection. Collection stays manual so
// only the demo worker drives it, one bounded range at a time.
func registerDemoSource(svc *service.Service) {
	if svc.Sources == nil {
		svc.Sources = map[string]service.MetricSource{}
	}
	svc.Sources[demoSource] = service.MetricSource{Kind: "synthetic", CollectionMode: "manual", Collector: demoTelemetry{}}
}

// collectDemo collects the typed demo SLOs for (from, to].
func collectDemo(ctx context.Context, svc *service.Service, from, to time.Time) error {
	for _, s := range demoTypedSLOs() {
		if _, err := svc.Collect(ctx, s.ID, from, to); err != nil {
			return err
		}
	}
	return nil
}
