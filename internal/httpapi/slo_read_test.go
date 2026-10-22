package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

type observedReadStore struct {
	store.Store
	reads        atomic.Int64
	lists        atomic.Int64
	changeOnRead bool
}

func (s *observedReadStore) Read(ctx context.Context, id string) (model.Record, error) {
	s.reads.Add(1)
	r, err := s.Store.Read(ctx, id)
	if err == nil && s.changeOnRead {
		r.SLO.Target = .8
		r.SLO.Revision = 2
	}
	return r, err
}
func (s *observedReadStore) List(ctx context.Context) ([]model.SLO, error) {
	s.lists.Add(1)
	return s.Store.List(ctx)
}

func TestSingleSLOReadUsesOnlySelectedTenantRecord(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Minute)
	workspaces := []Workspace{}
	observed := []*observedReadStore{}
	for i, name := range []string{"alpha", "beta"} {
		st := &observedReadStore{Store: store.NewMemory()}
		observed = append(observed, st)
		svc := service.New(st, nil)
		svc.Now = func() time.Time { return at }
		if err := svc.Create(ctx, model.SLO{ID: "checkout", Name: name, Project: "p", Service: "api", Target: .9, WindowSeconds: 60, StepSeconds: 60}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Ingest(ctx, "checkout", []model.Point{{Time: at, Good: []float64{99, 80}[i], Total: 100}}); err != nil {
			t.Fatal(err)
		}
		workspaces = append(workspaces, Workspace{ID: name, Name: name, Service: svc, Grants: []tenancy.Grant{{Token: name, Role: "viewer"}}})
	}
	h, err := NewMulti(workspaces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	request := func(tenant, token, path string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Nines-Tenant", tenant)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return w
	}
	for i, name := range []string{"alpha", "beta"} {
		var view service.SLOView
		if err := json.Unmarshal(request(name, name, "/api/slos/checkout", 200).Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.SLO.Name != name || view.SLO.Revision != 1 || view.Status.Reliability == nil || *view.Status.Reliability != []float64{.99, .8}[i] {
			t.Fatal(view)
		}
		if observed[i].reads.Load() != 1 || observed[i].lists.Load() != 0 {
			t.Fatal("point read scanned the inventory or read more than once")
		}
	}
	request("beta", "alpha", "/api/slos/checkout", 403)
	request("alpha", "unknown", "/api/slos/checkout", 401)
	if observed[0].reads.Load() != 1 || observed[1].reads.Load() != 1 {
		t.Fatal("denied request reached tenant storage")
	}
	request("alpha", "alpha", "/api/slos/missing", 404)
	var status model.Snapshot
	if err := json.Unmarshal(request("alpha", "alpha", "/api/slos/checkout/status", 200).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Reliability == nil || *status.Reliability != .99 {
		t.Fatal("specific status route was shadowed")
	}
}
func TestInventoryPairsCurrentDefinitionWithMatchingStatus(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Minute)
	st := &observedReadStore{Store: store.NewMemory(), changeOnRead: true}
	svc := service.New(st, nil)
	svc.Now = func() time.Time { return at }
	if err := svc.Create(ctx, model.SLO{ID: "checkout", Name: "Checkout", Project: "p", Service: "api", Target: .9, WindowSeconds: 60, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ingest(ctx, "checkout", []model.Point{{Time: at, Good: 90, Total: 100}}); err != nil {
		t.Fatal(err)
	}
	h := New(svc, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/slos", nil))
	var got []service.SLOView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got) != 1 || got[0].SLO.Revision != 2 || got[0].SLO.Target != .8 || got[0].Status.BudgetAllowed == nil || math.Abs(*got[0].Status.BudgetAllowed-20) > 1e-9 {
		t.Fatal(w.Code, got)
	}
	if st.reads.Load() != 1 || st.lists.Load() != 1 {
		t.Fatal("unnecessary reads")
	}
}
