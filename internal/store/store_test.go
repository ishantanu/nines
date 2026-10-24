package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { contract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); contract(t, st) })
}
func postgres(t *testing.T) (*store.Postgres, string) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(model.NewID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	st, err := store.NewPostgres(ctx, u.String())
	if err != nil {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Fatal(err)
		}
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Close()
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return st, u.String()
}
func contract(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	c := model.SLO{ID: "slo", Project: "p", Service: "api", Name: "test", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
	if err := st.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, c); !errors.Is(err, model.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := st.Read(ctx, "missing"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal(err)
	}
	rollback := errors.New("abort")
	err := st.Update(ctx, c.ID, func(r *model.Record) error {
		r.Points[at.Unix()] = model.Point{Time: at, Good: 1, Total: 1}
		r.Events = append(r.Events, model.AlertEvent{ID: "rolled-back", SLOID: c.ID, Time: at})
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	r, _ := st.Read(ctx, c.ID)
	ds, _ := st.Deliveries(ctx)
	if len(r.Points) != 0 || len(ds) != 0 {
		t.Fatal("transaction did not roll back")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			key := at.Add(time.Duration(i) * time.Minute)
			if err := st.Update(ctx, c.ID, func(r *model.Record) error {
				r.Points[key.Unix()] = model.Point{Time: key, Good: 99, Total: 100}
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	r, err = st.Read(ctx, c.ID)
	if err != nil || len(r.Points) != 20 {
		t.Fatalf("lost concurrent updates: %d %v", len(r.Points), err)
	}
	err = st.Update(ctx, c.ID, func(r *model.Record) error {
		r.Alert.Phase = "firing"
		r.Events = append(r.Events, model.AlertEvent{ID: "one-event", SLOID: c.ID, Kind: "firing", Time: at})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var claims atomic.Int64
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			d, err := st.Claim(ctx, at, 30*time.Second)
			if err != nil {
				t.Error(err)
			}
			if d != nil {
				claims.Add(1)
			}
		})
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("event claimed %d times", claims.Load())
	}
	d, err := st.Claim(ctx, at.Add(31*time.Second), 30*time.Second)
	if err != nil || d == nil || d.Attempts != 2 {
		t.Fatal(d, err)
	}
	if err = st.Finish(ctx, d.Event.ID, 1, at, ""); err != nil {
		t.Fatal(err)
	}
	ds, _ = st.Deliveries(ctx)
	if ds[0].Status != "pending" {
		t.Fatal("stale lease holder modified event")
	}
	if err = st.Finish(ctx, d.Event.ID, 2, at, ""); err != nil {
		t.Fatal(err)
	}
	ds, _ = st.Deliveries(ctx)
	if ds[0].Status != "sent" {
		t.Fatal(ds)
	}
}
func TestPostgresRestartPreservesSamplesStateAndOutbox(t *testing.T) {
	st, connection := postgres(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	c := model.SLO{ID: "persistent", Project: "p", Service: "s", Name: "test", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
	if err := st.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ctx, c.ID, func(r *model.Record) error {
		r.Points[at.Unix()] = model.Point{Time: at, Good: 90, Total: 100}
		r.Alert = model.AlertState{Phase: "firing", LastEvaluated: &at}
		r.Events = append(r.Events, model.AlertEvent{ID: "persisted", SLOID: c.ID, Kind: "firing", Time: at})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := store.NewPostgres(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, err := reopened.Read(ctx, c.ID)
	if err != nil || len(r.Points) != 1 || r.Alert.Phase != "firing" {
		t.Fatal(r, err)
	}
	d, err := reopened.Claim(ctx, at, time.Minute)
	if err != nil || d == nil || d.Event.ID != "persisted" {
		t.Fatal(d, err)
	}
}

func TestSLOConfigurationCASPersistsWithoutReinterpretingSamples(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st store.Store
			var dsn string
			if backend == "postgres" {
				st, dsn = postgres(t)
			} else {
				st = store.NewMemory()
			}
			ctx := context.Background()
			svc := service.New(st, nil)
			at := time.Now().UTC().Truncate(time.Minute)
			svc.Now = func() time.Time { return at }
			c := model.SLO{ID: "config-test", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
			if err := svc.Create(ctx, c); err != nil {
				t.Fatal(err)
			}
			if err := svc.Ingest(ctx, c.ID, []model.Point{{Time: at, Good: 99, Total: 100}}); err != nil {
				t.Fatal(err)
			}
			d := model.DefinitionFromSLO(c)
			d.Spec.WindowSeconds = 7200
			d.Spec.Target = .995
			d.Spec.Policy = &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 900, BurnRate: 4, ForSeconds: 60, MinCoverage: 1}
			var successes atomic.Int64
			var writers sync.WaitGroup
			for range 6 {
				writers.Go(func() {
					_, err := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1)
					if err == nil {
						successes.Add(1)
					} else if !errors.Is(err, model.ErrRevisionConflict) {
						t.Error(err)
					}
				})
			}
			writers.Wait()
			if successes.Load() != 1 {
				t.Fatal(successes.Load())
			}
			read := st
			if dsn != "" {
				reopened, err := store.NewPostgres(ctx, dsn)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				read = reopened
			}
			rec, err := read.Read(ctx, c.ID)
			if err != nil || rec.SLO.Revision != 2 || rec.SLO.WindowSeconds != 7200 || rec.SLO.Policy.LongWindowSeconds != 900 || len(rec.Points) != 1 {
				t.Fatal(rec, err)
			}
			d.Spec.TotalQuery = "different"
			d.Spec.GoodQuery = "different-good"
			if edited, err := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 2); err != nil || edited.Revision != 3 {
				t.Fatal("measurement edit failed", edited, err)
			}
			current, err := read.Read(ctx, c.ID)
			if err != nil || len(current.Points) != 0 || current.Collection.State != "not_observed" {
				t.Fatal("old evidence reused", current, err)
			}
			archive, err := read.(store.SLORevisionStore).SLORevision(ctx, c.ID, 2)
			if err != nil || len(archive.Record.Points) != 1 || archive.Record.SLO.GoodQuery != "" || archive.Summary.IntervalSamples != 1 {
				t.Fatal("old evidence not archived", archive, err)
			}
			// Returned snapshots cannot mutate archived state.
			delete(archive.Record.Points, at.Unix())
			again, err := read.(store.SLORevisionStore).SLORevision(ctx, c.ID, 2)
			if err != nil || len(again.Record.Points) != 1 {
				t.Fatal("archive alias", again, err)
			}
			rows, err := read.(store.SLORevisionStore).SLORevisions(ctx, c.ID, 0, 1)
			if err != nil || len(rows) != 1 || rows[0].Revision != 2 {
				t.Fatal(rows, err)
			}
			rows, err = read.(store.SLORevisionStore).SLORevisions(ctx, c.ID, 2, 1)
			if err != nil || len(rows) != 1 || rows[0].Revision != 1 {
				t.Fatal(rows, err)
			}
		})
	}
}

func TestPostgresOperatorRouteSurvivesRestartAndDefinitionUpdates(t *testing.T) {
	st, dsn := postgres(t)
	ctx := context.Background()
	slo := model.SLO{ID: "operator-route", Project: "commerce", Service: "checkout", Name: "Checkout", Target: .99, WindowSeconds: 60, StepSeconds: 1}
	if err := st.Create(ctx, slo); err != nil {
		t.Fatal(err)
	}
	if err := st.Update(ctx, slo.ID, func(r *model.Record) error { r.OperatorRoute = "reviewed-route-digest"; return nil }); err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Update(ctx, slo.ID, func(r *model.Record) error { r.SLO.Revision = 2; return nil }); err != nil {
		t.Fatal(err)
	}
	record, err := reopened.Read(ctx, slo.ID)
	if err != nil || record.OperatorRoute != "reviewed-route-digest" || record.SLO.Revision != 2 {
		t.Fatal(record.OperatorRoute, err)
	}
}

func TestDefinitionBatchArchivesCommitAndAbortTogether(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			var st store.Store
			if pg {
				st, _ = postgres(t)
			} else {
				st = store.NewMemory()
			}
			ctx := context.Background()
			svc := service.New(st, nil)
			original := model.SLO{ID: "archive-batch", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
			if err := svc.Create(ctx, original); err != nil {
				t.Fatal(err)
			}
			changed := model.DefinitionFromSLO(original)
			changed.Spec.GoodQuery = "vector(99)"
			changed.Spec.TotalQuery = "vector(100)"
			batch := st.(store.DefinitionBatchStore)
			aborted := errors.New("abort after SLO edit")
			_, err := batch.DefinitionBatch(ctx, "abort", "digest", true, func(tx store.Store) (json.RawMessage, error) {
				_, err := service.New(tx, nil).ApplyDefinition(ctx, model.Definition{SLO: &changed}, 1)
				if err != nil {
					return nil, err
				}
				return nil, aborted
			})
			if !errors.Is(err, aborted) {
				t.Fatal(err)
			}
			current, _ := st.Read(ctx, original.ID)
			if model.ConfigRevision(current.SLO) != 1 {
				t.Fatal("aborted revision committed")
			}
			if _, err = st.(store.SLORevisionStore).SLORevision(ctx, original.ID, 1); !errors.Is(err, model.ErrNotFound) {
				t.Fatal("aborted archive committed", err)
			}
			_, err = batch.DefinitionBatch(ctx, "commit", "digest", true, func(tx store.Store) (json.RawMessage, error) {
				_, err := service.New(tx, nil).ApplyDefinition(ctx, model.Definition{SLO: &changed}, 1)
				return json.RawMessage(`{"ok":true}`), err
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = st.(store.SLORevisionStore).SLORevision(ctx, original.ID, 1); err != nil {
				t.Fatal("committed archive absent", err)
			}
		})
	}
}
