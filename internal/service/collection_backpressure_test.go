package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

type blockedCollection struct {
	started, release chan struct{}
	entered          sync.Once
}

func (c *blockedCollection) Collect(ctx context.Context, _ model.SLO, _, to time.Time) ([]model.Point, error) {
	c.entered.Do(func() { close(c.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.release:
		return []model.Point{{Time: to, Good: 99, Total: 100}}, nil
	}
}
func TestCollectionBackpressurePreservesInteractiveReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &blockedCollection{started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(source.release) })
	svc := service.New(store.NewMemory(), source)
	at := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return at }
	c := model.SLO{ID: "busy", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "good", TotalQuery: "total"}
	if err := svc.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { _, err := svc.Collect(ctx, c.ID, at.Add(-time.Minute), at); first <- err }()
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("source did not start")
	}
	second := make(chan error, 1)
	go func() { _, err := svc.Collect(ctx, c.ID, at.Add(-time.Minute), at); second <- err }()
	select {
	case err := <-second:
		if !errors.Is(err, model.ErrUnavailable) {
			t.Fatal("expected explicit backpressure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive collection queued behind blocked query")
	}
	read, cancelRead := context.WithTimeout(ctx, time.Second)
	defer cancelRead()
	if _, err := svc.Status(read, c.ID, time.Time{}); err != nil {
		t.Fatal("collection blocked status read", err)
	}
	release.Do(func() { close(source.release) })
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}
