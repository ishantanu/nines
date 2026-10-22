package investigation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/tenancy"
)

func runFixture(t *testing.T, mode bool) (*Service, context.Context, string) {
	s, ctx, dsn := fixture(t, mode)
	s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
	if _, err := s.Create(ctx, input()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "run-1", ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	return s, ctx, dsn
}
func runModes(t *testing.T, fn func(*testing.T, bool)) {
	for _, mode := range []bool{false, true} {
		name := "memory"
		if mode {
			name = "postgres"
		}
		t.Run(name, func(t *testing.T) { fn(t, mode) })
	}
}
func TestReadOnlyRunCheckpointSurvivesReopen(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, dsn := runFixture(t, mode)
		before, _ := s.Platform.Store.Read(ctx, "checkout")
		beforeBytes, _ := json.Marshal(before)
		w := Worker{Service: s}
		if found, err := w.WorkOne(ctx); err != nil || !found {
			t.Fatal(found, err)
		}
		first, err := s.ReadRun(ctx, input().ID, "run-1")
		if err != nil || first.Steps[0].Status != "completed" || first.Steps[1].Status != "pending" {
			t.Fatal(first, err)
		}
		original := string(first.Steps[0].Evidence.Context)
		if mode {
			s.Repository.Close()
			repo, err := NewPostgres(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			s = New(s.Platform, repo)
			s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return nil }
		}
		w = Worker{Service: s, execute: func(ctx context.Context, r v1.InvestigationRun, i int) (v1.InvestigationRunStep, error) {
			if i != 1 {
				t.Fatal("completed step repeated", i)
			}
			return s.executeReadStep(ctx, r, i)
		}}
		if _, err = w.WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		saved, err := s.ReadRun(ctx, input().ID, "run-1")
		if err != nil || saved.Status != "completed" || saved.Attempts != 2 || saved.Steps[0].Attempts != 1 || saved.Steps[1].Attempts != 1 || string(saved.Steps[0].Evidence.Context) != original {
			t.Fatal(saved, err)
		}
		if saved.Steps[1].Assessment.State != "insufficient_evidence" {
			t.Fatal("missing data became success", saved.Steps[1])
		}
		parent, _ := s.Read(ctx, input().ID)
		if parent.Version != 1 || len(parent.Entries) != 0 {
			t.Fatal("run mutated conversation", parent)
		}
		record, _ := s.Platform.Store.Read(ctx, "checkout")
		afterBytes, _ := json.Marshal(record)
		if string(afterBytes) != string(beforeBytes) {
			t.Fatal("read-only run mutated SLO")
		}
		if found, err := w.WorkOne(ctx); err != nil || found {
			t.Fatal("terminal job executed", found, err)
		}
	})
}
func TestReadOnlyRunExpiredLeaseCannotCheckpoint(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		old, err := s.Repository.claimRun(ctx, "old-token")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Repository.updateRun(ctx, input().ID, "run-1", func(r *runRecord) error {
			r.Until = r.Now.Add(-time.Second)
			r.Run.Steps[0].Status = "running"
			r.Run.Steps[0].Attempts = 1
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		fresh, err := s.Repository.claimRun(ctx, "new-token")
		if err != nil || fresh.Run.Attempts != 2 {
			t.Fatal(fresh, err)
		}
		if _, err = s.Repository.updateRun(ctx, input().ID, "run-1", func(r *runRecord) error {
			if err := fenced(r, old.Token); err != nil {
				return err
			}
			finish(r, "completed", "")
			return nil
		}); !errors.Is(err, errLeaseLost) {
			t.Fatal("expired worker committed", err)
		}
		if _, err = s.Repository.updateRun(ctx, input().ID, "run-1", func(r *runRecord) error { r.Until = r.Now.Add(-time.Second); return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err = (&Worker{Service: s}).WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		saved, _ := s.ReadRun(ctx, input().ID, "run-1")
		if saved.Steps[0].Attempts != 2 || saved.Steps[0].Status != "completed" {
			t.Fatal(saved)
		}
	})
}
func TestReadOnlyRunCancellationFencesInflightResult(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		worker := Worker{Service: s, execute: func(ctx context.Context, r v1.InvestigationRun, i int) (v1.InvestigationRunStep, error) {
			close(entered)
			<-release
			return s.executeReadStep(ctx, r, i)
		}}
		go func() { _, err := worker.WorkOne(ctx); done <- err }()
		<-entered
		if found, err := (&Worker{Service: s}).WorkOne(ctx); err != nil || found {
			t.Fatal("concurrent worker obtained live lease", found, err)
		}
		current, _ := s.ReadRun(ctx, input().ID, "run-1")
		other := tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "alpha", Role: "editor", Subject: "other", Kind: "user"})
		if _, err := s.CancelRun(other, input().ID, "run-1", v1.CancelInvestigationRun{ExpectedVersion: current.Version}); !errors.Is(err, ErrForbidden) {
			t.Fatal(err)
		}
		if _, err := s.CancelRun(ctx, input().ID, "run-1", v1.CancelInvestigationRun{ExpectedVersion: 1}); !errors.Is(err, ErrConflict) {
			t.Fatal("stale cancellation accepted", err)
		}
		if _, err := s.CancelRun(ctx, input().ID, "run-1", v1.CancelInvestigationRun{ExpectedVersion: current.Version}); err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		saved, _ := s.ReadRun(ctx, input().ID, "run-1")
		if saved.Status != "cancelled" || saved.Steps[0].Evidence != nil || saved.CancelledBy.Subject != "user-1" {
			t.Fatal("cancelled result was published", saved)
		}
		if _, err := s.CancelRun(ctx, input().ID, "run-1", v1.CancelInvestigationRun{ExpectedVersion: current.Version}); err != nil {
			t.Fatal("cancellation retry failed", err)
		}
	})
}
func TestReadOnlyRunRetriesAndAuthorizationAreBounded(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		w := Worker{Service: s, execute: func(context.Context, v1.InvestigationRun, int) (v1.InvestigationRunStep, error) {
			return v1.InvestigationRunStep{}, errors.New("provider secret must not leak")
		}}
		for range 3 {
			if _, err := w.WorkOne(ctx); err != nil {
				t.Fatal(err)
			}
		}
		saved, _ := s.ReadRun(ctx, input().ID, "run-1")
		b, _ := json.Marshal(saved)
		if saved.Status != "failed" || saved.ErrorCode != "evidence_unavailable" || saved.Steps[0].Attempts != 3 || strings.Contains(string(b), "provider secret must not leak") {
			t.Fatal(saved)
		}
		if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "run-2", ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
		s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return ErrForbidden }
		if _, err := (&Worker{Service: s}).WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		denied, _ := s.ReadRun(ctx, input().ID, "run-2")
		if denied.Status != "failed" || denied.ErrorCode != "access_revoked" || denied.Steps[0].Attempts != 0 {
			t.Fatal(denied)
		}
		if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "run-3", ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Repository.updateRun(ctx, input().ID, "run-3", func(r *runRecord) error { r.Run.DeadlineAt = r.Now.Add(-time.Second); return nil })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = (&Worker{Service: s}).WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		expired, _ := s.ReadRun(ctx, input().ID, "run-3")
		if expired.ErrorCode != "deadline_exceeded" {
			t.Fatal(expired)
		}
	})
}
func TestReadOnlyRunEnqueueIsIdempotentAndSerialized(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		if _, err := s.Message(ctx, input().ID, v1.AppendInvestigationMessage{ID: "later", ExpectedVersion: 1, Text: "later"}); err != nil {
			t.Fatal(err)
		}
		replay, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "run-1", ExpectedVersion: 1})
		if err != nil || replay.ReviewVersion != 1 {
			t.Fatal("exact replay failed", err)
		}
		if _, err = s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "run-1", ExpectedVersion: 2}); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		if _, err = s.CancelRun(ctx, input().ID, "run-1", v1.CancelInvestigationRun{ExpectedVersion: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "stale", ExpectedVersion: 1}); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, id := range []string{"a", "b"} {
			wg.Go(func() {
				_, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: id, ExpectedVersion: 2})
				results <- err
			})
		}
		wg.Wait()
		close(results)
		accepted := 0
		for err := range results {
			if err == nil {
				accepted++
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
		}
		if accepted != 1 {
			t.Fatal("multiple active jobs", accepted)
		}
	})
}
func TestReadOnlyRunRejectsChangedResourceRevision(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		if err := s.Platform.Store.Update(ctx, "checkout", func(r *model.Record) error { r.SLO.Revision = 2; return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err := (&Worker{Service: s}).WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		r, _ := s.ReadRun(ctx, input().ID, "run-1")
		if r.Status != "failed" || r.ErrorCode != "scope_revision_conflict" || r.Steps[0].Evidence != nil {
			t.Fatal(r)
		}
	})
}
func TestAssessmentPreservesNoTrafficAndBoundedJourney(t *testing.T) {
	c := v1.SLOContext{Resource: v1.ResourceSummary{ID: "checkout"}, Measurement: v1.Measurement{Coverage: 1, DataStatus: "no_traffic", Target: .99}}
	check := func(scope v1.InvestigationScope, value any) *v1.EvidenceAssessment {
		b, _ := json.Marshal(value)
		digest := sha256.Sum256(b)
		out, err := assess(scope, v1.InvestigationEvidence{Context: b, Digest: hex.EncodeToString(digest[:])})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := check(v1.InvestigationScope{Kind: v1.SLOKind}, c); out.State != "insufficient_evidence" {
		t.Fatal(out)
	}
	reliability := .98
	c.Measurement.Reliability = &reliability
	c.Measurement.DataStatus = "complete"
	if out := check(v1.InvestigationScope{Kind: v1.SLOKind}, c); out.State != "window_below_target" {
		t.Fatal(out)
	}
	c.MeasurementHealth = &v1.BindingHealth{EvaluationBlocked: true}
	if out := check(v1.InvestigationScope{Kind: v1.SLOKind}, c); out.State != "insufficient_evidence" {
		t.Fatal("drift was treated as a valid target comparison", out)
	}
	c.MeasurementHealth = nil
	journey := v1.JourneyContext{Links: []v1.LinkedSLOContext{{Context: &c}}, NextCursor: "more"}
	if out := check(v1.InvestigationScope{Kind: v1.JourneyKind}, journey); out.State != "insufficient_evidence" || len(out.Findings) != 1 {
		t.Fatal(out)
	}
	if _, err := assess(v1.InvestigationScope{Kind: v1.SLOKind}, v1.InvestigationEvidence{Context: json.RawMessage(`{}`), Digest: "wrong"}); err == nil {
		t.Fatal("tampered evidence accepted")
	}
}

func TestReadOnlyRunRechecksAccessBetweenSteps(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		w := Worker{Service: s}
		if _, err := w.WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		s.AuthorizeRun = func(context.Context, v1.InvestigationActor) error { return ErrForbidden }
		if _, err := w.WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		r, _ := s.ReadRun(ctx, input().ID, "run-1")
		if r.Status != "failed" || r.ErrorCode != "access_revoked" || r.Steps[0].Evidence == nil || r.Steps[1].Attempts != 0 || r.Steps[1].Assessment != nil {
			t.Fatal(r)
		}
	})
}
func TestReadOnlyReviewRunLimit(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		for i := 0; i < 20; i++ {
			id := "run-1"
			if i > 0 {
				id = fmt.Sprintf("run-%d", i+1)
				if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: id, ExpectedVersion: 1}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.CancelRun(ctx, input().ID, id, v1.CancelInvestigationRun{ExpectedVersion: 1}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.CreateRun(ctx, input().ID, v1.CreateInvestigationRun{ID: "overflow", ExpectedVersion: 1}); err == nil {
			t.Fatal("run limit ignored")
		}
		list, err := s.ListRuns(ctx, input().ID)
		if err != nil || len(list.Items) != 20 {
			t.Fatal(len(list.Items), err)
		}
	})
}

func TestInterruptedStepAttemptLimitFailsBeforeAnotherRead(t *testing.T) {
	runModes(t, func(t *testing.T, mode bool) {
		s, ctx, _ := runFixture(t, mode)
		if _, err := s.Repository.updateRun(ctx, input().ID, "run-1", func(r *runRecord) error { r.Run.Steps[0].Attempts = 3; r.Run.Steps[0].Status = "running"; return nil }); err != nil {
			t.Fatal(err)
		}
		w := Worker{Service: s, execute: func(context.Context, v1.InvestigationRun, int) (v1.InvestigationRunStep, error) {
			t.Fatal("attempt limit allowed another read")
			return v1.InvestigationRunStep{}, nil
		}}
		if _, err := w.WorkOne(ctx); err != nil {
			t.Fatal(err)
		}
		r, _ := s.ReadRun(ctx, input().ID, "run-1")
		if r.Status != "failed" || r.ErrorCode != "attempt_limit" {
			t.Fatal(r)
		}
	})
}
