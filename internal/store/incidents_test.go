package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestIncidentStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { incidentContract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); incidentContract(t, st) })
}

func incidentContract(t *testing.T, st store.IncidentStore) {
	ctx := context.Background()
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	put := func(i model.Incident) error {
		return st.UpdateIncidents(ctx, func([]model.Incident) ([]model.Incident, error) { return []model.Incident{i}, nil })
	}
	first := model.Incident{ID: "a", Revision: 1, Title: "A", Source: "manual", State: model.IncidentDetected, OpenedAt: at, SLOs: []model.IncidentSLO{}, Journeys: []model.IncidentJourney{}, Timeline: []model.IncidentEvent{}}
	if err := put(first); err != nil {
		t.Fatal(err)
	}
	// Creating it again, or writing a revision that skips one, conflicts.
	if err := put(first); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("duplicate create accepted", err)
	}
	skipped := first
	skipped.Revision = 3
	if err := put(skipped); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("skipped revision accepted", err)
	}
	resolved := first
	resolved.Revision, resolved.State = 2, model.IncidentResolved
	if err := put(resolved); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID, second.OpenedAt = "b", at.Add(time.Hour)
	if err := put(second); err != nil {
		t.Fatal(err)
	}
	// Only open incidents reach the callback; listing is newest first.
	if err := st.UpdateIncidents(ctx, func(open []model.Incident) ([]model.Incident, error) {
		if len(open) != 1 || open[0].ID != "b" {
			t.Fatalf("open = %+v", open)
		}
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	list, err := st.Incidents(ctx, 10)
	if err != nil || len(list) != 2 || list[0].ID != "b" || list[1].State != model.IncidentResolved {
		t.Fatal(list, err)
	}
	if _, err = st.Incident(ctx, "missing"); !errors.Is(err, model.ErrNotFound) {
		t.Fatal(err)
	}
	// Concurrent callers are serialized: each sees the others' writes, so
	// exactly one of them opens an incident for the same journey.
	var wg sync.WaitGroup
	opened := make(chan string, 8)
	for n := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.UpdateIncidents(ctx, func(open []model.Incident) ([]model.Incident, error) {
				for _, i := range open {
					if i.Title == "journey" {
						return nil, nil
					}
				}
				opened <- string(rune('c' + n))
				return []model.Incident{{ID: string(rune('c' + n)), Revision: 1, Title: "journey", State: model.IncidentDetected, OpenedAt: at}}, nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(opened)
	if len(opened) != 1 {
		t.Fatalf("%d concurrent callers opened an incident", len(opened))
	}
}
