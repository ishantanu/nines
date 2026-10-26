package main

import (
	"sync"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/slotemplates"
)

type objective struct {
	Template string
	SLO      v1.ServiceLevelObjective
	Degraded float64
}

var fixtureDefinitions = sync.OnceValue(buildDefinitions)

// Definitions are immutable after the shared template renderer constructs them.
func definitions() []objective { return fixtureDefinitions() }

func buildDefinitions() []objective {
	cases := []struct {
		template, id string
		ratio        float64
	}{
		{"agent-task-completion", "ai-agent-tasks", .7}, {"agent-tool-success", "ai-agent-tools", 1}, {"agent-budget-compliance", "ai-agent-budget", .9},
		{"agent-evaluation-pass", "ai-agent-quality", .9}, {"llm-response-success", "ai-llm-responses", .8}, {"llm-evaluation-pass", "ai-llm-quality", .9},
		{"inference-availability", "ai-inference-availability", .8}, {"inference-latency", "ai-inference-latency", .8}, {"inference-ttft", "ai-inference-ttft", .5},
	}
	window, step := int64(60), int64(10)
	out := make([]objective, 0, len(cases)+1)
	for _, c := range cases {
		t, err := slotemplates.Get(c.template)
		if err != nil {
			panic(err)
		}
		measurement := t.Manifest.Spec.Measurement
		measurement.ScrapeIntervalSeconds = 5
		draft, err := slotemplates.Render(c.template, v1.TemplateValues{Name: c.id, Project: "ai-playground", WindowSeconds: &window, StepSeconds: &step, Measurement: measurement})
		if err != nil {
			panic(err)
		} // Checked-in fixture contracts must remain renderable.
		slo := draft.Manifest
		m := slo.Spec.Measurement
		m.ScrapeIntervalSeconds = 5
		if c.template == "agent-evaluation-pass" || c.template == "llm-evaluation-pass" {
			m.Labels["rubric"] = "fixture-v1"
			m.Labels["evaluator"] = "deterministic-v1"
			m.ObservationPoint = "Synthetic selected-cohort terminal pass, fail and unknown verdicts under fixture-v1 and deterministic-v1; only pass is good. Missing selected evaluations are measured separately. This fixture does not assess real model outputs."
		}
		out = append(out, objective{c.template, slo, c.ratio})
	}
	coverage := v1.NewSLO("ai-evaluation-coverage", "ai-playground", v1.SLOSpec{DisplayName: "Agent evaluation completion coverage", Service: "agent", Source: "primary", Lifecycle: v1.LifecycleExperimental, Target: .99, WindowSeconds: window, StepSeconds: step, Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "agent_evaluation_selections_total", Labels: map[string]string{"service": "agent", "rubric": "fixture-v1", "evaluator": "deterministic-v1"}, SuccessLabels: map[string]string{"completion": "completed"}, CountFrom: "counter", ScrapeIntervalSeconds: 5, ObservationPoint: "One classified completion or missing result per selected synthetic evaluation after its evaluation deadline. Missing selections stay in the denominator. This measures completion coverage, not verdict quality."}})
	out = append(out, objective{"", coverage, .6})
	return out
}
func journey() v1.CriticalUserJourney {
	defs := definitions()
	links := make([]v1.JourneySLO, 0, len(defs))
	for _, d := range defs {
		dimension := "correctness"
		if d.SLO.Spec.Measurement.Type == "latency" {
			dimension = "latency"
		} else if d.SLO.Metadata.Name == "ai-agent-budget" {
			dimension = "cost"
		} else if d.SLO.Metadata.Name == "ai-inference-availability" || d.SLO.Metadata.Name == "ai-llm-responses" || d.SLO.Metadata.Name == "ai-agent-tools" {
			dimension = "availability"
		}
		links = append(links, v1.JourneySLO{ID: d.SLO.Metadata.Name, Dimension: dimension, Required: true})
	}
	return v1.CriticalUserJourney{APIVersion: v1.APIVersion, Kind: v1.JourneyKind, Metadata: v1.Metadata{Name: "ai-support-resolution", Project: "ai-playground"}, Spec: v1.JourneySpec{DisplayName: "Resolve a support request with an agent", Description: "Synthetic workload linking serving reliability, usable responses, tool attempts, task outcomes and evaluation coverage. Does not run an actual agent or model.", Owner: "playground", Criticality: "high", SLOs: links, Steps: []v1.JourneyStep{
		{ID: "serve", Name: "Serve inference", Actor: "service", SLORefs: []string{"ai-inference-availability", "ai-inference-latency", "ai-inference-ttft"}},
		{ID: "generate", Name: "Return a usable answer", Actor: "agent", DependsOn: []string{"serve"}, SLORefs: []string{"ai-llm-responses", "ai-llm-quality"}},
		{ID: "act", Name: "Use tools within budget", Actor: "agent", DependsOn: []string{"generate"}, SLORefs: []string{"ai-agent-tools", "ai-agent-budget"}},
		{ID: "resolve", Name: "Complete and evaluate the task", Actor: "agent", DependsOn: []string{"act"}, SLORefs: []string{"ai-agent-tasks", "ai-agent-quality", "ai-evaluation-coverage"}},
	}}}
}
