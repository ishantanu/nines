package service

import (
	"context"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
)

func TestBudgetDecisionEvidenceAndPolicyBoundaries(t *testing.T) {
	s := setup(t)
	c := model.SLO{ID: "api", Target: .99, StepSeconds: 60, WindowSeconds: 60, ErrorBudgetPolicy: &v1.ErrorBudgetPolicy{Owner: "payments-oncall", RunbookURL: "https://example.test/runbooks/api", ReviewBelow: .25}}
	for _, tc := range []struct {
		name    string
		good    float64
		quality string
		state   string
	}{{"healthy", 10000, "usable", "proceed"}, {"half_budget", 9950, "usable", "proceed"}, {"review_boundary", 9925, "usable", "review_required"}, {"exhausted", 9900, "usable", "blocked"}, {"below_budget", 9800, "usable", "blocked"}, {"partial", 10000, "limited", "insufficient_evidence"}, {"missing", 10000, "unavailable", "insufficient_evidence"}} {
		t.Run(tc.name, func(t *testing.T) {
			x := engine.Evaluate(c, map[int64]model.Point{now.Unix(): {Time: now, Good: tc.good, Total: 10000}}, now, 60)
			q := v1.MeasurementQuality{State: tc.quality, DefinitionDigest: "definition", SourceDigest: "source", ObservedAt: now, Checks: []v1.QualityCheck{{Code: "coverage", State: "warning"}}}
			got := s.budgetFromEvidence(c, x, q, false)
			if got.State != tc.state || !got.Advisory || got.DefinitionDigest != "definition" || got.Owner != "payments-oncall" {
				t.Fatal(got)
			}
		})
	}
	x := engine.Evaluate(c, map[int64]model.Point{now.Unix(): {Time: now, Good: 9900, Total: 10000}}, now, 60)
	q := v1.MeasurementQuality{State: "usable", ObservedAt: now}
	c.ErrorBudgetPolicy.OnExhausted = "review_required"
	if got := s.budgetFromEvidence(c, x, q, false); got.State != "review_required" {
		t.Fatal(got)
	}
	c.Lifecycle = v1.LifecycleExperimental
	if got := s.budgetFromEvidence(c, x, q, false); got.State == "proceed" || got.ReasonCodes[0] != "objective_not_committed" {
		t.Fatal(got)
	}
	if got := s.budgetFromEvidence(c, x, q, true); got.State != "insufficient_evidence" {
		t.Fatal("historical evidence authorized current action", got)
	}
	c.ErrorBudgetPolicy = nil
	if got := s.budgetFromEvidence(c, x, q, false); got.State != "not_configured" {
		t.Fatal(got)
	}
}
func TestBudgetDecisionDoesNotPromoteStaleObservedSuccess(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error {
		r.SLO.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := s.Ingest(ctx, "availability", []model.Point{{Time: now.Add(-time.Duration(i) * time.Minute), Good: 100, Total: 100}}); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.ErrorBudgetDecision(ctx, "availability", 1)
	if err != nil || d.State != "proceed" {
		t.Fatal(d, err)
	}
	s.Now = func() time.Time { return now.Add(3 * time.Minute) }
	d, err = s.ErrorBudgetDecision(ctx, "availability", 1)
	if err != nil || d.State != "insufficient_evidence" {
		t.Fatal(d, err)
	}
}
