package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestJourneyStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { journeyContract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); journeyContract(t, st) })
}
func journeyContract(t *testing.T, st store.JourneyStore) {
	t.Helper()
	ctx := context.Background()
	j, err := model.ParseJourney([]byte(examples.AgentResolution))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	input := model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: now}
	first, err := st.ApplyJourney(ctx, input, 0)
	if err != nil || first.Revision != 1 {
		t.Fatal(first, err)
	}
	// Mutating a returned manifest cannot mutate persistent state.
	first.Manifest.Spec.Steps[0].Name = "mutation"
	read, err := st.JourneyRevision(ctx, j.Metadata.Name, 0)
	if err != nil || read.Manifest.Spec.Steps[0].Name == "mutation" {
		t.Fatal(read, err)
	}
	same, err := st.ApplyJourney(ctx, input, 1)
	if err != nil || same.Revision != 1 {
		t.Fatal("idempotency", same, err)
	}
	if _, err = st.ApplyJourney(ctx, input, 0); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale create", err)
	}
	j.Spec.Description = "edited"
	next := model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: now.Add(time.Second)}
	var winners atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			_, err := st.ApplyJourney(ctx, next, 1)
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, model.ErrRevisionConflict) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("concurrent writers won", winners.Load())
	}
	// Reverting content produces revision 3, preventing ABA compare-and-swap bugs.
	reverted, err := st.ApplyJourney(ctx, input, 2)
	if err != nil || reverted.Revision != 3 {
		t.Fatal(reverted, err)
	}
	history, err := st.JourneyHistory(ctx, j.Metadata.Name)
	if err != nil || len(history) != 3 || history[1].Manifest.Spec.Description != "edited" {
		t.Fatal(history, err)
	}
	cost := .1
	steps := 5
	approval := true
	e := model.RunEvidence{ID: "retry", JourneyRevision: 1, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "success", CostUSD: &cost, Steps: &steps, ApprovalObserved: &approval}
	run, err := model.EvaluateRun(j.Metadata.Name, history[0], e, now)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := st.PutRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	run.ReceivedAt = now.Add(time.Hour)
	again, err := st.PutRun(ctx, run)
	if err != nil || !again.ReceivedAt.Equal(saved.ReceivedAt) {
		t.Fatal("retry changed original evidence", again, err)
	}
	run.Evidence.Outcome = "failure"
	if _, err = st.PutRun(ctx, run); !errors.Is(err, model.ErrRunConflict) {
		t.Fatal("overwrote immutable run", err)
	}
	runs, err := st.Runs(ctx, j.Metadata.Name)
	if err != nil || len(runs) != 1 || runs[0].ContractDigest != history[0].Digest || runs[0].Evidence.JourneyRevision != 1 {
		t.Fatal(runs, err)
	}
}
func TestPostgresRestartPreservesJourneysAndRuns(t *testing.T) {
	st, connection := postgres(t)
	journeyContract(t, st)
	st.Close()
	reopened, err := store.NewPostgres(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	h, err := reopened.JourneyHistory(context.Background(), "assisted-resolution")
	if err != nil || len(h) != 3 {
		t.Fatal(h, err)
	}
	r, err := reopened.Runs(context.Background(), "assisted-resolution")
	if err != nil || len(r) != 1 || r[0].Verdict != "passed" {
		t.Fatal(r, err)
	}
}
