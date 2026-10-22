package model_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
)

func TestJourneyStrictParsingAndGraph(t *testing.T) {
	j, err := model.ParseJourney([]byte(examples.Checkout))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(j)
	if _, err = model.ParseJourney(b); err != nil {
		t.Fatal("JSON manifest", err)
	}
	for name, text := range map[string]string{
		"unknown field":      strings.Replace(examples.Checkout, "criticality: critical", "criticality: critical\n  critcality: high", 1),
		"duplicate key":      strings.Replace(examples.Checkout, "criticality: critical", "criticality: critical\n  criticality: high", 1),
		"multiple documents": examples.Checkout + "\n---\n{}",
		"json duplicate":     `{"apiVersion":"a","apiVersion":"b"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := model.ParseJourney([]byte(text)); err == nil {
				t.Fatal("accepted malformed manifest")
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*model.Journey)
	}{
		{"cycle", func(j *model.Journey) { j.Spec.Steps[0].DependsOn = []string{"send-confirmation"} }},
		{"unknown dependency", func(j *model.Journey) { j.Spec.Steps[0].DependsOn = []string{"missing"} }},
		{"duplicate step", func(j *model.Journey) { j.Spec.Steps[1].ID = j.Spec.Steps[0].ID }},
		{"undeclared reference", func(j *model.Journey) { j.Spec.Steps[0].SLORefs = []string{"missing"} }},
		{"unused link", func(j *model.Journey) {
			j.Spec.SLOs = append(j.Spec.SLOs, model.JourneySLO{ID: "unused", Dimension: "safety"})
		}},
		{"no required objectives", func(j *model.Journey) {
			for i := range j.Spec.SLOs {
				j.Spec.SLOs[i].Required = false
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var next model.Journey
			_ = json.Unmarshal(b, &next)
			tc.edit(&next)
			if err := next.Validate(); err == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
	// Key order and YAML formatting do not produce spurious resource revisions.
	again, _ := model.ParseJourney(b)
	if model.JourneyDigest(j) != model.JourneyDigest(again) {
		t.Fatal("unstable canonical digest")
	}
}
func TestAgenticEvidenceDoesNotInventSuccess(t *testing.T) {
	j, err := model.ParseJourney([]byte(examples.AgentResolution))
	if err != nil {
		t.Fatal(err)
	}
	rev := model.JourneyRevision{Manifest: j, Revision: 3, Digest: model.JourneyDigest(j)}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cost := .1
	steps := 5
	yes := true
	base := model.RunEvidence{ID: "run-1", JourneyRevision: 3, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "success", CostUSD: &cost, Steps: &steps, ApprovalObserved: &yes}
	for _, tc := range []struct {
		name, want string
		edit       func(*model.RunEvidence)
	}{
		{"complete success", "passed", func(*model.RunEvidence) {}},
		{"missing approval", "incomplete", func(e *model.RunEvidence) { e.ApprovalObserved = nil }},
		{"missing cost", "incomplete", func(e *model.RunEvidence) { e.CostUSD = nil }},
		{"missing steps", "incomplete", func(e *model.RunEvidence) { e.Steps = nil }},
		{"failed task", "failed", func(e *model.RunEvidence) { e.Outcome = "failure" }},
		{"handoff", "inconclusive", func(e *model.RunEvidence) { e.Outcome = "handoff" }},
		{"cost exceeded", "violated", func(e *model.RunEvidence) { n := .5; e.CostUSD = &n }},
		{"unapproved", "violated", func(e *model.RunEvidence) { n := false; e.ApprovalObserved = &n }},
		{"too many steps", "violated", func(e *model.RunEvidence) { n := 13; e.Steps = &n }},
		{"duration exceeded", "violated", func(e *model.RunEvidence) { e.StartedAt = now.Add(-121 * time.Second) }},
		{"safety violation", "violated", func(e *model.RunEvidence) { e.PolicyViolations = []string{"restricted tool used"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.edit(&e)
			r, err := model.EvaluateRun("agent", rev, e, now)
			if err != nil || r.Verdict != tc.want {
				t.Fatalf("%s: %+v %v", tc.want, r, err)
			}
		})
	}
	base.FinishedAt = now.Add(time.Second)
	if _, err = model.EvaluateRun("agent", rev, base, now); err == nil {
		t.Fatal("future evidence accepted")
	}
}
