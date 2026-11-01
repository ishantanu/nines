package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestVisualReceiptStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { visualContract(t, store.NewMemory(), "") })
	t.Run("postgres", func(t *testing.T) { st, url := postgres(t); visualContract(t, st, url) })
}
func visualContract(t *testing.T, st store.Store, url string) {
	ctx := context.Background()
	repo := st.(store.VisualStore)
	now := time.Now().UTC()
	r := v1.VisualEvaluation{ID: model.NewID(), Tenant: "default", JourneyID: "checkout", RequestDigest: "original-request", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Assessment: v1.VisualAssessment{Status: "pending"}}
	var created atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, new, e := repo.ReserveVisual(ctx, r)
			if e != nil {
				t.Error(e)
			}
			if new {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 {
		t.Fatal("assessment reserved more than once", created.Load())
	}
	changed := r
	changed.RequestDigest = "changed"
	if _, _, e := repo.ReserveVisual(ctx, changed); !errors.Is(e, model.ErrConflict) {
		t.Fatal("ID rebound", e)
	}
	if _, e := repo.UpdateVisual(ctx, r.ID, func(out *v1.VisualEvaluation) error {
		out.Assessment.Status = "assessed"
		out.Review = &v1.VisualReview{Outcome: "inconclusive", Reason: "Manual review"}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if url != "" {
		reopened, e := store.NewPostgres(ctx, url)
		if e != nil {
			t.Fatal(e)
		}
		defer reopened.Close()
		repo = reopened
	}
	out, e := repo.ReadVisual(ctx, r.ID)
	if e != nil || out.Assessment.Status != "assessed" || out.Review.Outcome != "inconclusive" {
		t.Fatal("receipt did not survive", out, e)
	}
	if rows, e := repo.ListVisual(ctx, "checkout"); e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	expired := r
	expired.ID = model.NewID()
	expired.ExpiresAt = now.Add(-time.Hour)
	if _, _, e := repo.ReserveVisual(ctx, expired); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.ReadVisual(ctx, expired.ID); !errors.Is(e, model.ErrNotFound) {
		t.Fatal("expired receipt visible", e)
	}
	if e := repo.(interface{ PruneVisual(context.Context) error }).PruneVisual(ctx); e != nil {
		t.Fatal(e)
	}
	if rows, e := repo.ListVisual(ctx, "checkout"); e != nil || len(rows) != 1 {
		t.Fatal("retention removed active receipt", rows, e)
	}
}
