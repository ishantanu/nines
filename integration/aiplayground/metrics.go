package main

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"sync"
)

var scenarios = []string{"healthy", "degraded", "task-failure", "evaluation-gap", "budget-unknown", "inference-failure", "slow-first-token", "no-traffic"}
var boundaries = []float64{.1, .3, .5, 1, 2, 5}

type histogram struct {
	buckets [6]uint64
	count   uint64
	sum     float64
}

func (h *histogram) observe(value float64, n uint64) {
	h.count += n
	h.sum += value * float64(n)
	for i, b := range boundaries {
		if value <= b {
			h.buckets[i] += n
		}
	}
}

type workload struct {
	mu                    sync.Mutex
	scenario              string
	counters              map[string]uint64
	durations, firstToken histogram
}

func newWorkload() *workload {
	w := &workload{scenario: "healthy", counters: map[string]uint64{}}
	// Export every bounded outcome from startup, including zero counts.
	for _, c := range definitions() {
		if c.SLO.Spec.Measurement.Type == "availability" {
			m := c.SLO.Spec.Measurement
			for key, good := range m.SuccessLabels {
				for _, value := range []string{good, "fail", "unknown", "cancelled", "timeout", "rejected", "missing"} {
					w.counters[series(m.Metric, m.Labels, key, value)] = 0
				}
			}
		}
	}
	for _, value := range []string{"completed", "missing"} {
		w.counters[series("llm_evaluation_selections_total", map[string]string{"service": "llm", "rubric": "fixture-v1", "evaluator": "deterministic-v1"}, "completion", value)] = 0
	}
	return w
}
func series(metric string, labels map[string]string, key, value string) string {
	names := make([]string, 0, len(labels)+1)
	for name := range labels {
		names = append(names, name)
	}
	names = append(names, key)
	slices.Sort(names)
	out := metric + "{"
	for i, name := range names {
		if i > 0 {
			out += ","
		}
		v := labels[name]
		if name == key {
			v = value
		}
		out += name + "=" + strconv.Quote(v)
	}
	return out + "}"
}
func (w *workload) tick() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.scenario == "no-traffic" {
		return
	}
	batch := map[string]map[string]uint64{
		"ai-agent-tasks": {"success": 100}, "ai-agent-tools": {"success": 200}, "ai-agent-budget": {"pass": 100},
		"ai-agent-quality": {"pass": 100}, "ai-evaluation-coverage": {"completed": 100}, "ai-llm-quality": {"pass": 100}, "ai-llm-responses": {"success": 100}, "ai-inference-availability": {"success": 100},
	}
	degraded := w.scenario == "degraded"
	if degraded || w.scenario == "task-failure" {
		batch["ai-agent-tasks"] = map[string]uint64{"success": 70, "fail": 20, "cancelled": 5, "timeout": 5}
	}
	if degraded || w.scenario == "evaluation-gap" {
		batch["ai-evaluation-coverage"] = map[string]uint64{"completed": 60, "missing": 40}
		batch["ai-agent-quality"] = map[string]uint64{"pass": 54, "fail": 3, "unknown": 3}
		batch["ai-llm-quality"] = map[string]uint64{"pass": 54, "fail": 3, "unknown": 3}
	}
	if degraded || w.scenario == "budget-unknown" {
		batch["ai-agent-budget"] = map[string]uint64{"pass": 90, "unknown": 10}
	}
	if degraded || w.scenario == "inference-failure" {
		batch["ai-inference-availability"] = map[string]uint64{"success": 80, "rejected": 10, "timeout": 5, "fail": 5}
		batch["ai-llm-responses"] = map[string]uint64{"success": 80, "fail": 20}
		duration := .2
		if degraded {
			duration = 1
		}
		w.durations.observe(duration, 80)
		w.durations.observe(3, 20)
	} else {
		duration := .2
		if w.scenario == "slow-first-token" {
			duration = 1
		}
		w.durations.observe(duration, 100)
	}
	firstTokens := batch["ai-inference-availability"]["success"]
	if degraded || w.scenario == "slow-first-token" {
		w.firstToken.observe(.1, firstTokens/2)
		w.firstToken.observe(.8, firstTokens-firstTokens/2)
	} else {
		w.firstToken.observe(.1, firstTokens)
	}
	for value, n := range batch["ai-evaluation-coverage"] {
		w.counters[series("llm_evaluation_selections_total", map[string]string{"service": "llm", "rubric": "fixture-v1", "evaluator": "deterministic-v1"}, "completion", value)] += n
	}
	for _, c := range definitions() {
		if values, ok := batch[c.SLO.Metadata.Name]; ok {
			m := c.SLO.Spec.Measurement
			for key := range m.SuccessLabels {
				for value, n := range values {
					w.counters[series(m.Metric, m.Labels, key, value)] += n
				}
			}
		}
	}
}
func (w *workload) metrics(out io.Writer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range definitions() {
		m := c.SLO.Spec.Measurement
		if m.Type == "availability" {
			_, _ = fmt.Fprintf(out, "# HELP %s Synthetic AI workload events, not real model evaluations.\n# TYPE %s counter\n", m.Metric, m.Metric)
		}
	}
	_, _ = fmt.Fprintln(out, "# HELP llm_evaluation_selections_total Synthetic selected LLM evaluation cohorts after their deadline.\n# TYPE llm_evaluation_selections_total counter")
	keys := make([]string, 0, len(w.counters))
	for key := range w.counters {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		_, _ = fmt.Fprintf(out, "%s %d\n", key, w.counters[key])
	}
	histogramMetrics(out, "inference_request_duration_seconds", w.durations)
	histogramMetrics(out, "inference_time_to_first_token_seconds", w.firstToken)
	_, _ = fmt.Fprintf(out, "# HELP nines_playground_scenario_info Current synthetic AI scenario.\n# TYPE nines_playground_scenario_info gauge\nnines_playground_scenario_info{scenario=%q} 1\n", w.scenario)
}
func histogramMetrics(out io.Writer, name string, h histogram) {
	_, _ = fmt.Fprintf(out, "# HELP %s Synthetic duration in seconds.\n# TYPE %s histogram\n", name, name)
	for i, b := range boundaries {
		_, _ = fmt.Fprintf(out, "%s_bucket{service=\"inference\",le=%q} %d\n", name, strconv.FormatFloat(b, 'g', -1, 64), h.buckets[i])
	}
	_, _ = fmt.Fprintf(out, "%s_bucket{service=\"inference\",le=\"+Inf\"} %d\n%s_count{service=\"inference\"} %d\n%s_sum{service=\"inference\"} %g\n", name, h.count, name, h.count, name, h.sum)
}
func (w *workload) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(out http.ResponseWriter, _ *http.Request) { out.WriteHeader(200) })
	mux.HandleFunc("GET /metrics", func(out http.ResponseWriter, _ *http.Request) {
		out.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.metrics(out)
	})
	mux.HandleFunc("GET /scenario", func(out http.ResponseWriter, _ *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		out.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(out, `{"scenario":%q}`, w.scenario)
	})
	mux.HandleFunc("POST /scenario", func(out http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if !slices.Contains(scenarios, name) || len(r.URL.Query()) != 1 {
			http.Error(out, "Choose a documented synthetic scenario", 400)
			return
		}
		w.mu.Lock()
		w.scenario = name
		w.mu.Unlock()
		out.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(out, `{"scenario":%q}`, name)
	})
	return mux
}
