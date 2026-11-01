package datasources

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/identity"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/store"
)

func TestManagedVictoriaMetricsRoutesPersistenceAndCapabilities(t *testing.T) {
	ctx := context.Background()
	repo, st := identity.NewMemory(), store.NewMemory()
	create := func() *Registry {
		r, err := New(ctx, repo, "alpha", nil, map[string]bool{"vm.example": true}, nil, base64.StdEncoding.EncodeToString(make([]byte, 32)), "", st, ProviderRoutes{"victoriametrics": {"42:7", "42:8"}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := create()
	in := Input{Name: "vm", Kind: "victoriametrics", URL: "https://vm.example", Credentials: map[string]string{"bearerToken": "synthetic-vm-secret"}, Options: map[string]string{"accountID": "42", "projectID": "7"}}
	public, err := r.Save(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if public["kind"] != "victoriametrics" || public["authentication"] != "bearer" || strings.Contains(string(encodeRecord(public)), "synthetic-vm-secret") {
		t.Fatal("provider metadata or credential boundary changed")
	}
	restarted := create()
	source, ok := restarted.Source("vm")
	if !ok || source.Kind != "victoriametrics" || source.Collector == nil {
		t.Fatal("VictoriaMetrics source lost after restart")
	}
	if _, ok := restarted.Target("vm"); ok {
		t.Fatal("unsupported rule publisher exposed")
	}
	if err = repo.View(ctx, func(tx identity.Tx) error {
		raw, e := tx.Get("data_source", "alpha", "vm")
		if strings.Contains(string(raw), "synthetic-vm-secret") {
			t.Fatal("plaintext credential persisted")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = st.Create(ctx, model.SLO{ID: "vm-objective", Source: "vm", Name: "VM availability", Project: "p", Service: "api", Target: .99, WindowSeconds: 300, StepSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	in.ExpectedVersion = 1
	in.Options = map[string]string{"accountID": "42", "projectID": "8"}
	if _, err = restarted.Save(ctx, in); err == nil {
		t.Fatal("referenced cluster route changed in place")
	}
	for _, bad := range []Input{
		{Name: "other", Kind: "victoriametrics", URL: in.URL, Options: map[string]string{"accountID": "43"}},
		{Name: "project-only", Kind: "victoriametrics", URL: in.URL, Options: map[string]string{"projectID": "7"}},
		{Name: "all", Kind: "victoriametrics", URL: in.URL, Options: map[string]string{"accountID": "multitenant"}},
		{Name: "path", Kind: "victoriametrics", URL: in.URL + "/select/42:7/prometheus"},
		{Name: "rules", Kind: "victoriametrics", URL: in.URL, RulesEnabled: true},
		{Name: "unauthorized", Kind: "victoriametrics", URL: "https://other.example"},
	} {
		if _, err = restarted.Save(ctx, bad); err == nil {
			t.Fatal("unsafe managed connection accepted", bad.Name)
		}
	}
	if _, err = restarted.Save(ctx, Input{Name: "single", Kind: "victoriametrics", URL: in.URL}); err != nil {
		t.Fatal("single-node connection rejected", err)
	}
}
