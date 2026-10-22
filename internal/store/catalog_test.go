package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestCatalogContract(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st store.Store = store.NewMemory()
			if backend == "postgres" {
				st, _ = postgres(t)
			}
			ctx := context.Background()
			catalog := st.(store.CatalogStore)
			journeys := st.(store.JourneyStore)
			for i := 0; i < 137; i++ {
				id := fmt.Sprintf("slo-%03d", i)
				if err := st.Create(ctx, model.SLO{ID: id, Project: "commerce", Name: "Checkout", Service: "api", GoodQuery: strings.Repeat("secret-query", 1000), Revision: 2}); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{"a-b", "a0", "aa", "slo-000"} {
				j := model.Journey{Metadata: model.JourneyMetadata{Name: id, Project: "commerce"}, Spec: model.JourneySpec{DisplayName: "Old matches", SLOs: []model.JourneySLO{{ID: "slo-000", Required: true}}}}
				r, err := journeys.ApplyJourney(ctx, model.JourneyRevision{Manifest: j, Digest: "old"}, 0)
				if err != nil {
					t.Fatal(err)
				}
				r.Manifest.Spec.DisplayName = "Checkout journey"
				r.Manifest.Metadata.Project = "payments"
				r.Digest = "new"
				if _, err = journeys.ApplyJourney(ctx, r, 1); err != nil {
					t.Fatal(err)
				}
			}
			linked, err := catalog.SearchResources(ctx, store.CatalogQuery{LinkedSLO: "slo-000", Limit: 2})
			if err != nil || len(linked) != 2 || linked[0].Kind != v1.JourneyKind {
				t.Fatal("bounded reverse link discovery failed", linked, err)
			}
			all := []v1.ResourceSummary{}
			afterKind, afterID := "", ""
			for {
				rows, err := catalog.SearchResources(ctx, store.CatalogQuery{AfterKind: afterKind, AfterID: afterID, Limit: 7})
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) > 7 {
					t.Fatal("unbounded page")
				}
				if len(rows) == 0 {
					break
				}
				for _, x := range rows {
					if x.Revision != 2 {
						t.Fatal(x)
					}
					key := x.Kind + "/" + x.ID
					if len(all) > 0 {
						prev := all[len(all)-1]
						if prev.Kind+"/"+prev.ID >= key {
							t.Fatal("nonmonotonic or duplicate key", key)
						}
					}
					all = append(all, x)
				}
				last := rows[len(rows)-1]
				afterKind, afterID = last.Kind, last.ID
			}
			if len(all) != 141 {
				t.Fatal("lost metadata", len(all))
			}
			rows, err := catalog.SearchResources(ctx, store.CatalogQuery{Project: "commerce", Kind: v1.JourneyKind, Limit: 10})
			if err != nil || len(rows) != 0 {
				t.Fatal("historical project escaped latest-revision filtering", rows, err)
			}
			rows, err = catalog.SearchResources(ctx, store.CatalogQuery{Query: "old matches", Limit: 10})
			if err != nil || len(rows) != 0 {
				t.Fatal("old journey title leaked", rows, err)
			}
			rows, err = catalog.SearchResources(ctx, store.CatalogQuery{Kind: v1.SLOKind, Query: "checkout", Limit: 3})
			if err != nil || len(rows) != 3 {
				t.Fatal(rows, err)
			}
			b, _ := json.Marshal(rows)
			if strings.Contains(string(b), "secret-query") {
				t.Fatal("catalog returned query text")
			}
			for _, literal := range []string{"%", "_", "' OR 1=1 --"} {
				rows, err = catalog.SearchResources(ctx, store.CatalogQuery{Query: literal, Limit: 10})
				if err != nil || len(rows) != 0 {
					t.Fatal("search interpreted syntax", literal, rows, err)
				}
			}
			// Collection health shares the same persistent transaction as measurements.
			stamp := time.Now().UTC().Truncate(time.Microsecond)
			if err = st.Update(ctx, "slo-000", func(r *model.Record) error {
				r.Collection = v1.CollectionHealth{State: "failed", AttemptedAt: stamp, ErrorCode: "provider_error", LastSuccessAt: stamp.Add(-time.Minute)}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			r, err := st.Read(ctx, "slo-000")
			if err != nil || r.Collection.State != "failed" || !r.Collection.AttemptedAt.Equal(stamp) {
				t.Fatal(r.Collection, err)
			}
		})
	}
}

func TestCollectionHealthUpgradeAndReopen(t *testing.T) {
	st, dsn := postgres(t)
	ctx := context.Background()
	if err := st.Create(ctx, model.SLO{ID: "legacy", Project: "p", Name: "Legacy", Service: "api"}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "ALTER TABLE slos DROP COLUMN collection_health"); err != nil {
		t.Fatal(err)
	}
	admin.Close()
	upgraded, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	r, err := upgraded.Read(ctx, "legacy")
	if err != nil || r.Collection.State != "not_observed" {
		t.Fatal("legacy row was not upgraded", r.Collection, err)
	}
	if err = upgraded.Update(ctx, "legacy", func(r *model.Record) error {
		r.Collection = v1.CollectionHealth{State: "failed", ErrorCode: "provider_error", Measurement: &v1.BindingHealth{State: "drifted", Code: "contract_changed", EvaluationBlocked: true, SLORevision: 3}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	reopened, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, err = reopened.Read(ctx, "legacy")
	if err != nil || r.Collection.State != "failed" || r.Collection.ErrorCode != "provider_error" || r.Collection.Measurement == nil || !r.Collection.Measurement.EvaluationBlocked || r.Collection.Measurement.State != "drifted" || r.Collection.Measurement.SLORevision != 3 {
		t.Fatal("collection state lost after reopen", r.Collection, err)
	}
}
