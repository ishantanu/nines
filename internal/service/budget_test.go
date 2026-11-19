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

func TestBudgetBandsShowTheAgreedResponseWithoutChangingTheDecisionState(t *testing.T) {
	s := setup(t)
	bands := []v1.BudgetBand{{Name: "watch", Below: .5, Response: "Prioritize reliability bugs."}, {Name: "protect", Below: .25, Response: "Halt risky launches."}}
	c := model.SLO{ID: "api", Target: .99, StepSeconds: 60, WindowSeconds: 60, ErrorBudgetPolicy: &v1.ErrorBudgetPolicy{Owner: "payments-oncall", RunbookURL: "https://example.test/runbooks/api", ReviewBelow: .25, Bands: bands}}
	usable := v1.MeasurementQuality{State: "usable", ObservedAt: now}
	for _, tc := range []struct {
		name  string
		good  float64
		state string
		band  string
	}{
		{"healthy", 10000, "proceed", ""},
		{"watch", 9940, "proceed", "watch"},             // 40% of the budget left
		{"protect", 9920, "review_required", "protect"}, // 20% left: reviewBelow is separate from bands
		{"exhausted", 9900, "blocked", "protect"},
	} {
		x := engine.Evaluate(c, map[int64]model.Point{now.Unix(): {Time: now, Good: tc.good, Total: 10000}}, now, 60)
		got := s.budgetFromEvidence(c, x, usable, false)
		band := ""
		if got.Band != nil {
			band = got.Band.Name
		}
		if got.State != tc.state || band != tc.band {
			t.Errorf("%s: state %q band %q, want %q %q", tc.name, got.State, band, tc.state, tc.band)
		}
	}
	// Bands never exist where the decision itself is not made.
	x := engine.Evaluate(c, map[int64]model.Point{now.Unix(): {Time: now, Good: 9900, Total: 10000}}, now, 60)
	if got := s.budgetFromEvidence(c, x, v1.MeasurementQuality{State: "limited", ObservedAt: now}, false); got.Band != nil || got.State != "insufficient_evidence" {
		t.Fatalf("insufficient evidence must not show a band: %+v", got)
	}
	c.Lifecycle = v1.LifecycleExperimental
	if got := s.budgetFromEvidence(c, x, usable, false); got.Band != nil {
		t.Fatalf("an uncommitted objective must not show a band: %+v", got)
	}
}
