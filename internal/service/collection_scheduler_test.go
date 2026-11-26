package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

type fairnessSource struct {
	active, max atomic.Int64
	delay       time.Duration
	calls       atomic.Int64
}

func (p *fairnessSource) Collect(ctx context.Context, c model.SLO, from, to time.Time) ([]model.Point, error) {
	p.calls.Add(1)
	active := p.active.Add(1)
	defer p.active.Add(-1)
	for before := p.max.Load(); active > before; before = p.max.Load() {
		if p.max.CompareAndSwap(before, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
	}
	points := []model.Point{}
	for at := from.Add(time.Duration(c.StepSeconds) * time.Second); !at.After(to); at = at.Add(time.Duration(c.StepSeconds) * time.Second) {
		points = append(points, model.Point{Time: at, Good: 99, Total: 100})
	}
	return points, nil
}
func scheduledTenant(t *testing.T, id string, count int, source Collector) *Service {
	t.Helper()
	s := New(store.NewMemory(), source)
	s.TenantID = id
	for i := 0; i < count; i++ {
		if err := s.Create(context.Background(), model.SLO{ID: fmt.Sprintf("slo-%03d", i), Project: "p", Service: "api", Name: "Test", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total"}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}
func TestCollectionSchedulerFairnessBoundsAndCancellation(t *testing.T) {
	source := &fairnessSource{delay: 40 * time.Millisecond}
	big := scheduledTenant(t, "big", 40, source)
	small := scheduledTenant(t, "small", 1, source)
	scheduler, err := NewCollectionScheduler([]*Service{big, small}, 2)
	if err != nil {
		t.Fatal(err)
	}
	scheduler.Poll = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	completed := []string{}
	smallDone := make(chan struct{})
	var once sync.Once
	scheduler.OnResult = func(r CollectionResult) {
		mu.Lock()
		completed = append(completed, r.Tenant)
		mu.Unlock()
		if r.Tenant == "small" {
			once.Do(func() { close(smallDone) })
		}
	}
	stopped := make(chan struct{})
	go func() { defer close(stopped); scheduler.Run(ctx, nil) }()
	select {
	case <-smallDone:
	case <-time.After(3 * time.Second):
		t.Fatal("small tenant starved")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("scheduler ignored cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	position := -1
	for i, name := range completed {
		if name == "small" {
			position = i
			break
		}
	}
	if position > 1 || source.max.Load() > 2 {
		t.Fatal("tenant fairness/concurrency violated", position, source.max.Load())
	}
}
func TestCollectionSchedulerCoalescesAcrossReplicas(t *testing.T) {
	source := &fairnessSource{delay: 20 * time.Millisecond}
	a := scheduledTenant(t, "tenant", 1, source)
	b := New(a.Store, source)
	b.TenantID = a.TenantID
	one, _ := NewCollectionScheduler([]*Service{a}, 1)
	two, _ := NewCollectionScheduler([]*Service{b}, 1)
	var group sync.WaitGroup
	var worked atomic.Int64
	for _, scheduler := range []*CollectionScheduler{one, two} {
		group.Go(func() {
			ok, err := scheduler.CollectOne(context.Background(), scheduler.Services[0])
			if err != nil {
				t.Error(err)
			}
			if ok {
				worked.Add(1)
			}
		})
	}
	group.Wait()
	if worked.Load() != 1 || source.calls.Load() != 1 {
		t.Fatal("duplicate scheduled provider query", worked.Load(), source.calls.Load())
	}
}

// A tenant added while the scheduler runs is collected, and a removed one is
// not collected again.
func TestCollectionSchedulerAddsAndRemovesTenantsWhileRunning(t *testing.T) {
	source := &fairnessSource{delay: time.Millisecond}
	first := scheduledTenant(t, "first", 1, source)
	scheduler, err := NewCollectionScheduler([]*Service{first}, 2)
	if err != nil {
		t.Fatal(err)
	}
	scheduler.Poll = 5 * time.Millisecond
	var mu sync.Mutex
	seen := map[string]int{}
	scheduler.OnResult = func(r CollectionResult) {
		mu.Lock()
		seen[r.Tenant]++
		mu.Unlock()
	}
	count := func(tenant string) int {
		mu.Lock()
		defer mu.Unlock()
		return seen[tenant]
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { defer close(stopped); scheduler.Run(ctx, nil) }()

	added := scheduledTenant(t, "added", 1, source)
	if err = scheduler.Add(added); err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Add(added); err != nil || len(scheduler.services()) != 2 {
		t.Fatal("adding a tenant twice must schedule it once", err, len(scheduler.services()))
	}
	deadline := time.Now().Add(3 * time.Second)
	for count("added") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the added tenant was never collected")
		}
		time.Sleep(5 * time.Millisecond)
	}
	scheduler.Remove(added)
	if services := scheduler.services(); len(services) != 1 || services[0] != first {
		t.Fatal("the removed tenant is still scheduled")
	}
	// Let a job that was already running finish, then nothing more starts.
	time.Sleep(50 * time.Millisecond)
	after := count("added")
	time.Sleep(100 * time.Millisecond)
	if count("added") != after {
		t.Fatal("a removed tenant was collected again")
	}
	cancel()
	<-stopped
}
