package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/store"
)

type CollectionResult struct {
	Tenant, ID       string
	DueAt, StartedAt time.Time
	Duration         time.Duration
	Failed           bool
}

// CollectionScheduler limits scheduled work across tenants. A replica has at
// most one active job per tenant; replicas coordinate individual SLOs in storage.
type CollectionScheduler struct {
	// Services are the tenants collected. Change them while running with
	// Add and Remove.
	Services    []*Service
	Concurrency int
	Poll        time.Duration
	OnResult    func(CollectionResult)
	mu          sync.Mutex
}

// Add schedules a tenant created while the scheduler runs.
func (w *CollectionScheduler) Add(s *Service) error {
	if _, ok := s.Store.(store.CollectionQueue); !ok {
		return errors.New("store does not support durable collection scheduling")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if slices.Contains(w.Services, s) {
		return nil
	}
	w.Services = append(slices.Clone(w.Services), s)
	return nil
}

// Remove stops scheduling a tenant, such as one that was retired. A job
// already running finishes.
func (w *CollectionScheduler) Remove(s *Service) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Services = slices.DeleteFunc(slices.Clone(w.Services), func(x *Service) bool { return x == s })
}

func (w *CollectionScheduler) services() []*Service {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Services
}

func NewCollectionScheduler(services []*Service, concurrency int) (*CollectionScheduler, error) {
	if concurrency < 1 || concurrency > 32 {
		return nil, model.Invalid("collection workers must be between 1 and 32")
	}
	for _, s := range services {
		if _, ok := s.Store.(store.CollectionQueue); !ok {
			return nil, errors.New("store does not support durable collection scheduling")
		}
	}
	return &CollectionScheduler{Services: services, Concurrency: concurrency, Poll: time.Second}, nil
}
func (w *CollectionScheduler) CollectOne(ctx context.Context, s *Service) (bool, error) {
	queue, ok := s.Store.(store.CollectionQueue)
	if !ok {
		return false, model.ErrUnavailable
	}
	discovery, stop := context.WithTimeout(ctx, 5*time.Second)
	configs, err := s.Store.List(discovery)
	ids := []string{}
	for _, c := range configs {
		if !c.Retired() {
			ids = append(ids, c.ID)
		}
	}
	var job *store.CollectionJob
	if err == nil {
		job, err = queue.ClaimCollection(discovery, ids, time.Minute)
	}
	stop()
	if err != nil || job == nil {
		return false, err
	}
	start := time.Now()
	observed, finish := observability.Operation(ctx, "collector.scheduled", s.TenantID)
	run, cancel := context.WithTimeout(store.WithCollectionLease(observed, *job), 45*time.Second)
	record, err := s.Store.Read(run, job.ID)
	if err == nil && !record.SLO.Retired() {
		if s.AutomaticCollection(record.SLO) {
			var from, to time.Time
			from, to, err = s.scheduledCollectionRange(run, record.SLO)
			if err == nil {
				_, err = s.Collect(run, job.ID, from, to)
			}
		}
		// A source error does not suppress unknown/stale evaluation. The same lease
		// fences alert transitions as well as sample and collection-health writes.
		evaluationErr := s.Evaluate(run, job.ID)
		if err == nil {
			err = evaluationErr
		}
	}
	cancel()
	delay := collectionInterval
	if err != nil {
		delay = min(5*time.Minute, time.Duration(1<<min(job.Failures, 6))*5*time.Second)
	}
	completion, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	completeErr := queue.FinishCollection(completion, *job, delay, err != nil)
	stop()
	if err == nil {
		err = completeErr
	}
	finish(err)
	observability.CollectionCompleted(ctx, s.TenantID, time.Since(start), max(time.Duration(0), start.Sub(job.DueAt)), err != nil)
	if w.OnResult != nil {
		w.OnResult(CollectionResult{Tenant: s.TenantID, ID: job.ID, DueAt: job.DueAt, StartedAt: start, Duration: time.Since(start), Failed: err != nil})
	}
	return true, err
}
func (w *CollectionScheduler) Run(ctx context.Context, log *slog.Logger) {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if w.Concurrency < 1 || w.Poll <= 0 {
		return
	}
	tick := time.NewTicker(w.Poll)
	defer tick.Stop()
	type completion struct {
		service *Service
		worked  bool
		err     error
	}
	done := make(chan completion, w.Concurrency)
	active := map[*Service]bool{}
	next := map[*Service]time.Time{}
	running, cursor := 0, 0
	var group sync.WaitGroup
	defer group.Wait()
	launch := func() {
		services := w.services()
		for scanned := 0; scanned < len(services) && running < w.Concurrency; scanned++ {
			cursor %= len(services)
			s := services[cursor]
			cursor++
			if active[s] || time.Now().Before(next[s]) {
				continue
			}
			active[s] = true
			running++
			group.Add(1)
			go func() {
				defer group.Done()
				worked, err := w.CollectOne(ctx, s)
				done <- completion{s, worked, err}
			}()
		}
	}
	launch()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			launch()
		case result := <-done:
			delete(active, result.service)
			running--
			if !result.worked || result.err != nil {
				next[result.service] = time.Now().Add(w.Poll)
			} else {
				delete(next, result.service)
			}
			if result.err != nil && ctx.Err() == nil {
				log.Warn("scheduled collection needs attention", "tenant", result.service.TenantID)
			}
			launch()
		}
	}
}
