package intentflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func TestInlineIntentHumanConfirmationApplyAndRollback(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, dsn := fixture(t, pg)
			var database store.Store = store.NewMemory()
			if pg {
				p, e := store.NewPostgres(ctx, dsn)
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(p.Close)
				database = p
			}
			s.Platform = service.New(database, &testtelemetry.Source{})
			s.Platform.TenantID = "alpha"
			in := input()
			in.Scope.Journey = nil
			in.Goal.Spec.Journey = nil
			in.Goal.Spec.StepSeconds = 60
			in.Goal.Spec.Measurement = &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "Completed API requests"}
			before := v1.NewSLO(in.Scope.Metadata.Name, "test", v1.SLOSpec{DisplayName: in.Scope.Metadata.Name, Service: "api", Source: "primary", Target: .99, WindowSeconds: 3600, StepSeconds: 60, Measurement: in.Goal.Spec.Measurement})
			if _, e := s.Platform.ApplyDefinition(ctx, model.Definition{SLO: (*model.SLODefinition)(&before)}, 0); e != nil {
				t.Fatal(e)
			}
			collectFreshBaseline(t, s, ctx, in.Scope.Metadata.Name)
			v, e := s.Create(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discover-inline", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 1)
			v, e = s.Read(ctx, v.ID)
			if e != nil || v.State != "needs_selection" || v.Plan.Compiled == nil {
				t.Fatal(v, e)
			}
			confirmation := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true}
			agent := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Subject: "agent", Role: "agent", Kind: "api_token"})
			if _, e = s.Select(agent, v.ID, confirmation); !errors.Is(e, ErrForbidden) {
				t.Fatal("agent confirmed", e)
			}
			wrong := confirmation
			wrong.MeasurementDigest = strings.Repeat("0", 64)
			if _, e = s.Select(ctx, v.ID, wrong); e == nil {
				t.Fatal("wrong measurement confirmed")
			}
			v, e = s.Select(ctx, v.ID, confirmation)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "compile-inline", ExpectedVersion: v.Version, Mode: "guided"})
			if e != nil {
				t.Fatal(e)
			}
			work(t, s, ctx, 2)
			v, e = s.Read(ctx, v.ID)
			if e != nil || v.State != "ready" {
				t.Fatal(v.State, e)
			}
			v, e = s.PrepareChangeSet(ctx, v.ID, v.Version)
			if e != nil {
				t.Fatal(e)
			}
			if v.ChangeSet.Recommendation.MeasurementDigest != confirmation.MeasurementDigest {
				t.Fatal("lost measurement provenance")
			}
			v, e = s.ApproveChangeSet(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Reviewed measurement and target impact"))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ApplyChangeSet(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Digest, ""))
			if e != nil {
				t.Fatal(e)
			}
			applied, _ := s.Platform.Store.Read(ctx, in.Scope.Metadata.Name)
			if applied.SLO.Target != .999 || applied.SLO.Revision != 2 || applied.SLO.MeasurementBinding != nil {
				t.Fatal(applied)
			}
			v, e = s.PrepareRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Digest, "Restore prior target"))
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ReviewRollback(ctx, v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, "Reviewed baseline"), false)
			if e != nil {
				t.Fatal(e)
			}
			v, e = s.ReviewRollback(independentApplier(ctx), v.ID, reviewInput(v, v.ChangeSet.Rollback.Digest, ""), true)
			if e != nil {
				t.Fatal(e)
			}
			restored, _ := s.Platform.Store.Read(ctx, in.Scope.Metadata.Name)
			if restored.SLO.Target != .99 || restored.SLO.Revision != 3 || restored.SLO.Measurement == nil {
				t.Fatal(restored)
			}
			audit, e := s.Audit(ctx, v.ID)
			if e != nil || audit.Integrity != "verified" {
				t.Fatal(audit, e)
			}
			entries, e := database.(store.TopologyStore).ListTopology(context.Background(), v1.BindingKind, "", 100)
			if e != nil || len(entries) != 0 {
				t.Fatal("created hidden binding", entries, e)
			}
		})
	}
}

func TestInlineMeasurementEditInvalidatesConfirmation(t *testing.T) {
	s, ctx, _ := fixture(t, false)
	in := input()
	in.Scope.Journey = nil
	in.Goal.Spec.Journey = nil
	in.Goal.Spec.StepSeconds = 60
	in.Goal.Spec.Measurement = &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "API completions"}
	v, e := s.Create(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.Start(ctx, v.ID, v1.CreateIntentFlowRun{ID: "discover-inline", ExpectedVersion: v.Version, Mode: "guided"})
	if e != nil {
		t.Fatal(e)
	}
	work(t, s, ctx, 1)
	v, _ = s.Read(ctx, v.ID)
	confirmation := v1.ConfirmIntentBinding{ExpectedVersion: v.Version, IntentDigest: v.Plan.IntentDigest, MeasurementDigest: v.Plan.Compiled.MeasurementDigest, Confirm: true}
	v, e = s.Select(ctx, v.ID, confirmation)
	if e != nil {
		t.Fatal(e)
	}
	in.Goal.Spec.Measurement.ObservationPoint = "Gateway completions, including retries"
	v, e = s.Append(ctx, v.ID, v1.AppendIntentConversation{ID: "edit-measurement", ExpectedVersion: v.Version, Text: "Correct observation point", Goal: in.Goal})
	if e != nil || v.Selection != nil || v.Plan != nil || v.Backtest != nil {
		t.Fatal("measurement edit retained review evidence", v, e)
	}
	confirmation.ExpectedVersion = v.Version
	if _, e = s.Select(ctx, v.ID, confirmation); !errors.Is(e, ErrConflict) {
		t.Fatal("stale inline confirmation accepted", e)
	}
}
