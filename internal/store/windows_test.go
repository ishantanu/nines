package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestProviderWindowStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { windowContract(t, store.NewMemory()) })
	t.Run("postgres", func(t *testing.T) { st, _ := postgres(t); windowContract(t, st) })
}
func windowContract(t *testing.T, st store.Store) {
	ctx := context.Background()
	at := time.Unix(1800000000, 0).UTC()
	s := model.SLO{ID: "windows", Revision: 3, ProviderWindow: &v1.ProviderWindow{GoodSelector: "good", TotalSelector: "total", ScrapeIntervalSeconds: 2}}
	if err := st.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	o := model.WindowObservation{Time: at, WindowSeconds: 120, Revision: 3, Good: 82, Total: 100, Coverage: .9}
	key := model.WindowKey(120, at)
	if err := st.Update(ctx, s.ID, func(r *model.Record) error {
		r.Windows = map[string]model.WindowObservation{key: o}
		r.Events = append(r.Events, model.AlertEvent{ID: "window-fire", SLOID: s.ID, Time: at, Kind: "firing"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r, err := st.Read(ctx, s.ID)
	if err != nil || r.Windows[key] != o || len(r.Points) != 0 {
		t.Fatal(r, err)
	}
	abort := errors.New("abort")
	if err = st.Update(ctx, s.ID, func(r *model.Record) error { delete(r.Windows, key); return abort }); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	r, _ = st.Read(ctx, s.ID)
	if r.Windows[key] != o {
		t.Fatal("window transaction did not roll back")
	}
	if err = st.Update(ctx, s.ID, func(r *model.Record) error { o.Coverage = 1; r.Windows[key] = o; return nil }); err != nil {
		t.Fatal(err)
	}
	r, _ = st.Read(ctx, s.ID)
	if r.Windows[key].Coverage != 1 {
		t.Fatal("upsert failed")
	}
	if err = st.Update(ctx, s.ID, func(r *model.Record) error { delete(r.Windows, key); return nil }); err != nil {
		t.Fatal(err)
	}
	r, _ = st.Read(ctx, s.ID)
	if len(r.Windows) != 0 {
		t.Fatal("delete failed")
	}
	events, _ := st.Deliveries(ctx)
	if len(events) != 1 || events[0].Event.Kind != "firing" {
		t.Fatal(events)
	}
}
