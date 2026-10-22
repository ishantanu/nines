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

func TestManagedSourcesEncryptPersistAndEnforceDatasetBounds(t *testing.T) {
	ctx := context.Background()
	repo := identity.NewMemory()
	st := store.NewMemory()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	create := func() *Registry {
		r, e := New(ctx, repo, "alpha", map[string]bool{"primary": true}, map[string]bool{"metrics.test": true}, map[string]bool{"alpha-org": true}, key, t.TempDir(), st)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := create()
	in := Input{Name: "prod", Kind: "mimir", URL: "https://metrics.test/prometheus", MimirOrgID: "alpha-org", BearerToken: "super-private-token", RulesEnabled: true}
	out, err := r.Save(ctx, in)
	if err != nil || out["version"] != int64(1) {
		t.Fatal(out, err)
	}
	if strings.Contains(string(encodeRecord(out)), in.BearerToken) {
		t.Fatal("API returned credential")
	}
	if err = repo.View(ctx, func(tx identity.Tx) error {
		raw, e := tx.Get("data_source", "alpha", "prod")
		if strings.Contains(string(raw), in.BearerToken) {
			t.Fatal("plaintext stored")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	restarted := create()
	if _, ok := restarted.Source("prod"); !ok {
		t.Fatal("source lost on restart")
	}
	target, ok := restarted.Target("prod")
	if !ok || target.BearerToken != in.BearerToken || target.MimirOrgID != "alpha-org" {
		t.Fatal("wrong target credentials/dataset")
	}
	in.ExpectedVersion = 1
	in.BearerToken = ""
	in.MimirOrgID = "beta-org"
	if _, e := r.Save(ctx, in); e == nil {
		t.Fatal("cross-organization source accepted")
	}
	in.MimirOrgID = "alpha-org"
	in.URL = "https://untrusted.test"
	if _, e := r.Save(ctx, in); e == nil {
		t.Fatal("unapproved host accepted")
	}
	slo := model.SLO{ID: "api", Source: "prod", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 60, WindowSeconds: 3600}
	if e := st.Create(ctx, slo); e != nil {
		t.Fatal(e)
	}
	if e := r.Delete(ctx, "prod", 1); e == nil {
		t.Fatal("referenced source deleted")
	}
	in.URL = "https://metrics.test/another"
	if _, e := r.Save(ctx, in); e == nil {
		t.Fatal("referenced dataset changed")
	}
	in.URL = "https://metrics.test/prometheus"
	in.BearerToken = "rotated-private-token"
	if _, e := r.Save(ctx, in); e != nil {
		t.Fatal("credential rotation rejected", e)
	}
}
