package store_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/examples"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/workflow"
)

func TestPostgresTenantSchemasIsolateAllResources(t *testing.T) {
	legacy, base := postgres(t)
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	defaultURL, err := store.TenantDatabaseURL(ctx, base, "default")
	if err != nil || defaultURL != base {
		t.Fatal("legacy default namespace changed", defaultURL, err)
	}
	if _, err = store.TenantDatabaseURL(ctx, base, "bad|tenant"); err == nil {
		t.Fatal("invalid tenant accepted")
	}
	type instance struct {
		url   string
		store *store.Postgres
		repo  workflow.Repository
		svc   *service.Service
		total float64
	}
	instances := []instance{}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for n, id := range []string{"alpha", "beta"} {
		scoped, err := store.TenantDatabaseURL(ctx, base, id)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(scoped)
		schema := u.Query().Get("search_path")
		cleanupPool, err := pgxpool.New(ctx, base)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := cleanupPool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
				t.Error(err)
				return
			}
			cleanupPool.Close()
		})
		st, err := store.NewPostgres(ctx, scoped)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(st.Close)
		repo, err := workflow.NewPostgres(ctx, scoped)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(repo.Close)
		svc := service.New(st, nil)
		svc.TenantID = id
		svc.Now = func() time.Time { return now }
		total := float64(100 + n)
		for _, sloID := range []string{"api-availability", "background-jobs"} {
			if err = svc.Create(ctx, model.SLO{ID: sloID, Name: id, Project: "shared-project", Service: "api", Target: .99, WindowSeconds: 60, StepSeconds: 60}); err != nil {
				t.Fatal(err)
			}
			if err = svc.Ingest(ctx, sloID, []model.Point{{Time: now, Good: 90, Total: total}}); err != nil {
				t.Fatal(err)
			}
		}
		j, _ := model.ParseJourney([]byte(examples.AgentResolution))
		j.Spec.Owner = id
		rev, err := svc.ApplyJourney(ctx, j, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.RecordRun(ctx, j.Metadata.Name, model.RunEvidence{ID: "shared-run", JourneyRevision: rev.Revision, StartedAt: now.Add(-time.Minute), FinishedAt: now, Outcome: "unknown"}); err != nil {
			t.Fatal(err)
		}
		j.Spec.Description = id + " edit"
		next := model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: now}
		if _, err = st.ApplyJourneyOnce(ctx, next, 1, "same-operation"); err != nil {
			t.Fatal("receipt crossed tenant", err)
		}
		if err = st.Update(ctx, "api-availability", func(r *model.Record) error {
			r.Events = append(r.Events, model.AlertEvent{ID: "same-event", SLOID: r.SLO.ID, Kind: id, Time: now})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		wf := workflow.New(svc, repo)
		if _, err = wf.Create(ctx, workflow.CreateRequest{ID: "shared-workflow", Kind: "investigate", Note: id}); err != nil {
			t.Fatal(err)
		}
		instances = append(instances, instance{scoped, st, repo, svc, total})
	}
	if instances[0].url == instances[1].url {
		t.Fatal("tenant namespaces alias")
	}
	for n, instance := range instances {
		instance.repo.Close()
		instance.store.Close()
		reopened, err := store.NewPostgres(ctx, instance.url)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		repo, err := workflow.NewPostgres(ctx, instance.url)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		record, err := reopened.Read(ctx, "api-availability")
		if err != nil || record.Points[now.Unix()].Total != instance.total {
			t.Fatal("sample leak", record, err)
		}
		rev, err := reopened.JourneyRevision(ctx, "assisted-resolution", 0)
		if err != nil || rev.Revision != 2 || rev.Manifest.Spec.Owner != []string{"alpha", "beta"}[n] {
			t.Fatal("CUJ/revision leak", rev, err)
		}
		runs, err := reopened.Runs(ctx, "assisted-resolution")
		if err != nil || len(runs) != 1 {
			t.Fatal("evidence leak", runs, err)
		}
		d, err := reopened.Claim(ctx, now, time.Minute)
		if err != nil || d == nil || d.Event.Kind != []string{"alpha", "beta"}[n] {
			t.Fatal("outbox leak", d, err)
		}
		w, err := repo.Get(ctx, "shared-workflow")
		if err != nil || w.Note != []string{"alpha", "beta"}[n] {
			t.Fatal("workflow leak", w, err)
		}
	}
	original, err := legacy.List(ctx)
	if err != nil || len(original) != 0 {
		t.Fatal("tenant data leaked into legacy namespace", original, err)
	}
}
