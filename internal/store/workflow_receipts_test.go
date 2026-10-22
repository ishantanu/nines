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

func TestJourneyApplyReceipts(t *testing.T) {
	t.Run("memory", func(t *testing.T) { receiptContract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); receiptContract(t, st) })
}
func receiptContract(t *testing.T, st interface {
	store.JourneyStore
	store.JourneyApplier
}) {
	t.Helper()
	ctx := context.Background()
	j, err := model.ParseJourney([]byte(examples.AgentResolution))
	if err != nil {
		t.Fatal(err)
	}
	input := model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: time.Now().UTC()}
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			r, err := st.ApplyJourneyOnce(ctx, input, 0, "approved-operation")
			if err != nil {
				t.Error(err)
			} else if r.Revision != 1 {
				t.Error("unexpected revision", r)
			} else {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 12 {
		t.Fatal("same-operation retries not idempotent", successes.Load())
	}
	history, err := st.JourneyHistory(ctx, j.Metadata.Name)
	if err != nil || len(history) != 1 {
		t.Fatal("duplicate apply", history, err)
	}
	changed := input
	changed.Manifest.Spec.Description = "different"
	changed.Digest = model.JourneyDigest(changed.Manifest)
	if _, err = st.ApplyJourneyOnce(ctx, changed, 0, "approved-operation"); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("receipt accepted different manifest", err)
	}
	if _, err = st.ApplyJourneyOnce(ctx, input, 1, "approved-operation"); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("receipt accepted changed expected revision", err)
	}
	if _, err = st.ApplyJourneyOnce(ctx, input, 0, "another-operation"); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("different writer claimed receipt", err)
	}
	if _, err = st.ApplyJourney(ctx, changed, 1); err != nil {
		t.Fatal(err)
	}
	retry, err := st.ApplyJourneyOnce(ctx, input, 0, "approved-operation")
	if err != nil || retry.Revision != 1 {
		t.Fatal("original receipt lost after later change", retry, err)
	}
	retry.Manifest.Spec.DisplayName = "tampered"
	again, err := st.ApplyJourneyOnce(ctx, input, 0, "approved-operation")
	if err != nil || again.Manifest.Spec.DisplayName == "tampered" {
		t.Fatal("mutable receipt", again, err)
	}
}
