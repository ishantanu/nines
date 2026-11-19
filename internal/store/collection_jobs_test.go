package store_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func TestCollectionLeaseFencesLateAndCrossTenantWrites(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var a, b store.Store
			if backend == "memory" {
				a, b = store.NewMemory(), store.NewMemory()
			} else {
				a, _ = postgres(t)
				b, _ = postgres(t)
			}
			ctx := context.Background()
			for _, st := range []store.Store{a, b} {
				if err := st.Create(ctx, model.SLO{ID: "slo", Project: "p", Service: "api", Name: "Test", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
					t.Fatal(err)
				}
			}
			queue := a.(store.CollectionQueue)
			first, err := queue.ClaimCollection(ctx, []string{"slo"}, time.Minute)
			if err != nil || first == nil {
				t.Fatal(first, err)
			}
			var claims atomic.Int64
			var group sync.WaitGroup
			for i := 0; i < 10; i++ {
				group.Go(func() {
					job, e := queue.ClaimCollection(ctx, []string{"slo"}, time.Minute)
					if e != nil {
						t.Error(e)
					}
					if job != nil {
						claims.Add(1)
					}
				})
			}
			group.Wait()
			if claims.Load() != 0 {
				t.Fatal("duplicate active owners")
			}
			other, err := b.(store.CollectionQueue).ClaimCollection(ctx, []string{"slo"}, time.Minute)
			if err != nil || other == nil {
				t.Fatal(other, err)
			}
			mutate := func(r *model.Record) error {
				r.Points[60] = model.Point{Time: time.Unix(60, 0), Good: 99, Total: 100}
				r.Events = append(r.Events, model.AlertEvent{ID: "stale-event", SLOID: "slo", Time: time.Now()})
				return nil
			}
			if err = b.Update(store.WithCollectionLease(ctx, *first), "slo", mutate); !errors.Is(err, store.ErrCollectionLease) {
				t.Fatal("cross-tenant lease accepted", err)
			}
			// Completed owners cannot commit after a replacement acquires the job.
			if err = queue.FinishCollection(ctx, *first, 0, false); err != nil {
				t.Fatal(err)
			}
			replacement, err := queue.ClaimCollection(ctx, []string{"slo"}, time.Minute)
			if err != nil || replacement == nil || replacement.Token == first.Token {
				t.Fatal(replacement, err)
			}
			if err = a.Update(store.WithCollectionLease(context.WithoutCancel(ctx), *first), "slo", mutate); !errors.Is(err, store.ErrCollectionLease) {
				t.Fatal("late sample/alert write accepted", err)
			}
			if err = queue.FinishCollection(ctx, *first, 0, false); !errors.Is(err, store.ErrCollectionLease) {
				t.Fatal("stale finish accepted", err)
			}
			record, _ := a.Read(ctx, "slo")
			events, _ := a.Deliveries(ctx)
			if len(record.Points) != 0 || len(events) != 0 {
				t.Fatal("late owner mutated samples or outbox")
			}
			if err = a.Update(store.WithCollectionLease(ctx, *replacement), "slo", mutate); err != nil {
				t.Fatal(err)
			}
			if err = queue.FinishCollection(ctx, *replacement, 15*time.Second, false); err != nil {
				t.Fatal(err)
			}
			if job, e := queue.ClaimCollection(ctx, []string{"slo"}, time.Minute); job != nil || e != nil {
				t.Fatal("completed job was immediately repeated", job, e)
			}
		})
	}
}

func TestCollectionLeaseExpiryDuringCommitRollsBack(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var st store.Store
			if backend == "memory" {
				st = store.NewMemory()
			} else {
				st, _ = postgres(t)
			}
			ctx := context.Background()
			if err := st.Create(ctx, model.SLO{ID: "slo", Target: .99, WindowSeconds: 3600, StepSeconds: 60}); err != nil {
				t.Fatal(err)
			}
			job, err := st.(store.CollectionQueue).ClaimCollection(ctx, []string{"slo"}, time.Second)
			if err != nil || job == nil {
				t.Fatal(err)
			}
			err = st.Update(store.WithCollectionLease(ctx, *job), "slo", func(r *model.Record) error {
				r.Points[60] = model.Point{Time: time.Unix(60, 0), Good: 1, Total: 1}
				time.Sleep(time.Until(job.LeaseUntil) + 20*time.Millisecond)
				return nil
			})
			if !errors.Is(err, store.ErrCollectionLease) {
				t.Fatal("expired transaction committed", err)
			}
			record, _ := st.Read(ctx, "slo")
			if len(record.Points) != 0 {
				t.Fatal("expired transaction retained data")
			}
		})
	}
}

type failoverSource struct {
	entered, release chan struct{}
	good             float64
}

func (f *failoverSource) Collect(_ context.Context, c model.SLO, from, to time.Time) ([]model.Point, error) {
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	points := []model.Point{}
	for at := from.Add(time.Duration(c.StepSeconds) * time.Second); !at.After(to); at = at.Add(time.Duration(c.StepSeconds) * time.Second) {
		points = append(points, model.Point{Time: at, Good: f.good, Total: 100})
	}
	return points, nil
}
func TestPostgresCollectionReplicaFailoverAndRestart(t *testing.T) {
	st, dsn := postgres(t)
	ctx := context.Background()
	peer, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	control, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	at := time.Now().UTC().Truncate(time.Minute)
	slow := &failoverSource{entered: make(chan struct{}), release: make(chan struct{}), good: 100}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(slow.release) }) }
	t.Cleanup(release)
	first := service.New(st, slow)
	first.Now = func() time.Time { return at }
	c := model.SLO{ID: "api", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 600, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total", Policy: &model.Policy{ShortWindowSeconds: 60, LongWindowSeconds: 120, BurnRate: 6, MinCoverage: 1}}
	if err = first.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err = st.Update(ctx, c.ID, func(r *model.Record) error {
		r.SLO.RuleDestination = "fixed-dataset"
		r.OperatorRoute = "fixed-operator-route"
		r.RuleAutomation = &v1.RuleAutomationStatus{Enabled: true, Paused: true, Revision: 1, Digest: "unchanged"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	original, _ := st.Read(ctx, c.ID)
	one, _ := service.NewCollectionScheduler([]*service.Service{first}, 1)
	oldDone := make(chan error, 1)
	go func() { _, e := one.CollectOne(ctx, first); oldDone <- e }()
	select {
	case <-slow.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first collector did not claim")
	}
	if _, err = control.Exec(ctx, "UPDATE collection_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE slo_id='api'"); err != nil {
		t.Fatal(err)
	}
	healthy := service.New(peer, &failoverSource{good: 0})
	healthy.Now = first.Now
	two, _ := service.NewCollectionScheduler([]*service.Service{healthy}, 1)
	if worked, e := two.CollectOne(ctx, healthy); !worked || e != nil {
		t.Fatal("replacement did not recover", worked, e)
	}
	release()
	select {
	case e := <-oldDone:
		if e == nil {
			t.Fatal("late collector committed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("late collector did not stop")
	}
	record, err := peer.Read(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Points) != 2 || record.Alert.Phase != "firing" {
		t.Fatal("replacement data or transition lost", len(record.Points), record.Alert)
	}
	for _, p := range record.Points {
		if p.Good != 0 {
			t.Fatal("late healthy response overwrote replacement")
		}
	}
	events, err := peer.Deliveries(ctx)
	if err != nil || len(events) != 1 {
		t.Fatal("duplicate or missing firing transition", events, err)
	}
	if record.OperatorRoute != original.OperatorRoute || record.SLO.RuleDestination != original.SLO.RuleDestination || model.DefinitionDigest(record.RuleAutomation) != model.DefinitionDigest(original.RuleAutomation) {
		t.Fatal("collection failover changed rule ownership")
	}
	st.Close()
	restarted, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restartService := service.New(restarted, &failoverSource{good: 0})
	restartService.Now = first.Now
	if _, err = control.Exec(ctx, "UPDATE collection_jobs SET due_at=clock_timestamp()-interval '1 second' WHERE slo_id='api'"); err != nil {
		t.Fatal(err)
	}
	three, _ := service.NewCollectionScheduler([]*service.Service{restartService}, 1)
	if _, err = three.CollectOne(ctx, restartService); err != nil {
		t.Fatal(err)
	}
	events, err = restarted.Deliveries(ctx)
	record, readErr := restarted.Read(ctx, c.ID)
	if err != nil || readErr != nil || len(events) != 1 || len(record.Points) != 2 {
		t.Fatal("restart duplicated samples or alert", len(events), len(record.Points), err, readErr)
	}
}
