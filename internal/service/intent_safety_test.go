package service

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestActionTimeSafetyRefreshesEvidenceAndProtectsRecovery(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	points := []model.Point{}
	for i := 0; i < 10; i++ {
		points = append(points, model.Point{Time: now.Add(-time.Duration(i) * time.Minute), Good: 100, Total: 100})
	}
	if err := s.Ingest(ctx, "availability", points); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error {
		r.SLO.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	check, err := s.intentSafety(ctx, "availability", 1, nil, false)
	if err != nil || check.State != "allowed" || check.Budget.State != "proceed" {
		t.Fatal(check, err)
	}
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error { r.Alert.Phase = "firing"; return nil }); err != nil {
		t.Fatal(err)
	}
	active, err := s.intentSafety(ctx, "availability", 1, nil, false)
	if err == nil || active.BurnState != "firing" {
		t.Fatal("active burn allowed", active, err)
	}
	if err := s.Store.Update(ctx, "availability", func(r *model.Record) error { r.Alert.Phase = "inactive"; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Ingest(ctx, "availability", []model.Point{{Time: now, Good: 90, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	exhausted, err := s.intentSafety(ctx, "availability", 1, nil, false)
	if err == nil || exhausted.Budget.State != "blocked" {
		t.Fatal("exhausted budget allowed", exhausted, err)
	}
	if err := s.Ingest(ctx, "availability", points); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return now.Add(5 * time.Minute) }
	check, err = s.intentSafety(ctx, "availability", 1, nil, false)
	var blocked *IntentSafetyError
	if !errors.As(err, &blocked) || check.State != "blocked" || check.Quality.Freshness.State != "stale" {
		t.Fatal(check, err)
	}
	recovery, err := s.intentSafety(ctx, "availability", 1, nil, true)
	if err != nil || recovery.State != "reviewed_rollback" || recovery.Quality.State == "usable" {
		t.Fatal(recovery, err)
	}
	exception := &v1.IntentSafetyException{Reason: "Restore a reviewed measurement after instrumentation failure", ApprovedBy: v1.InvestigationActor{Subject: "reviewer"}, ApprovedAt: s.Now(), ExpiresAt: s.Now().Add(30 * time.Minute)}
	for _, tc := range []struct {
		role, subject string
		allowed       bool
	}{{"owner", "applier", true}, {"editor", "applier", false}, {"agent", "applier", false}, {"owner", "reviewer", false}} {
		scoped := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: s.TenantID, Role: tc.role, Subject: tc.subject})
		c, e := s.intentSafety(scoped, "availability", 1, exception, false)
		if (e == nil) != tc.allowed || (tc.allowed && c.State != "exception") {
			t.Fatal(tc, c, e)
		}
	}
	exception.ExpiresAt = s.Now()
	if _, err = s.intentSafety(tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: s.TenantID, Role: "owner", Subject: "applier"}), "availability", 1, exception, false); err == nil {
		t.Fatal("expired exception allowed")
	}
	if _, err = s.intentSafety(ctx, "availability", 2, nil, true); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("recovery bypassed revision", err)
	}
	created, err := s.intentSafety(ctx, "new-objective", 0, nil, false)
	if err != nil || created.State != "initial_definition" || created.Quality != nil {
		t.Fatal(created, err)
	}
}
