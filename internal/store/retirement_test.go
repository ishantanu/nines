package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRetirementStoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { retirementContract(t, store.NewMemory(), "") })
	t.Run("postgres", func(t *testing.T) { st, url := postgres(t); retirementContract(t, st, url) })
}
func retirementContract(t *testing.T, st store.Store, url string) {
	ctx := tenancy.WithPrincipal(context.Background(), tenancy.Principal{TenantID: "default", Role: "owner", Kind: "local_user", Subject: "retirement-engineer"})
	svc := service.New(st, nil)
	now := time.Now().UTC().Truncate(time.Minute)
	svc.Now = func() time.Time { return now }
	c := model.SLO{ID: "retire-me", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 3600, StepSeconds: 60}
	if e := svc.Create(ctx, c); e != nil {
		t.Fatal(e)
	}
	if e := svc.Ingest(ctx, c.ID, []model.Point{{Time: now, Good: 99, Total: 100}}); e != nil {
		t.Fatal(e)
	}
	js := st.(store.JourneyStore)
	j := model.Journey{APIVersion: v1.APIVersion, Kind: v1.JourneyKind, Metadata: model.ResourceMetadata{Name: "journey", Project: "p"}, Spec: v1.JourneySpec{SLOs: []v1.JourneySLO{{ID: c.ID, Required: true}}}}
	revision := model.JourneyRevision{Manifest: j, Digest: model.JourneyDigest(j), AppliedAt: now}
	if _, e := js.ApplyJourney(ctx, revision, 0); e != nil {
		t.Fatal(e)
	}
	blocked, e := svc.PlanRetirement(ctx, c.ID, "archive")
	if e != nil || len(blocked.References) != 1 || !blocked.References[0].Required || len(blocked.Blockers) != 1 {
		t.Fatal(blocked, e)
	}
	if _, e = svc.ApplyRetirement(ctx, c.ID, v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: blocked.Digest}); e == nil {
		t.Fatal("dangling required reference accepted")
	}
	j.Spec.SLOs = nil
	revision.Manifest = j
	revision.Digest = model.JourneyDigest(j)
	if _, e = js.ApplyJourney(ctx, revision, 1); e != nil {
		t.Fatal(e)
	}
	plan, e := svc.PlanRetirement(ctx, c.ID, "archive")
	if e != nil {
		t.Fatal(e)
	}
	in := v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: plan.Digest}
	stale := in
	stale.ExpectedRevision = 2
	if _, e = svc.ApplyRetirement(ctx, c.ID, stale); !errors.Is(e, model.ErrRevisionConflict) {
		t.Fatal("stale retirement", e)
	}
	other := tenancy.WithPrincipal(context.Background(), tenancy.Principal{TenantID: "other", Role: "owner"})
	if _, e = svc.ApplyRetirement(other, c.ID, in); e == nil {
		t.Fatal("cross-tenant action accepted")
	}
	receipt, e := svc.ApplyRetirement(ctx, c.ID, in)
	if e != nil || receipt.Revision != 2 || receipt.Retirement.Cleanup != "complete" {
		t.Fatal(receipt, e)
	}
	replay, e := svc.ApplyRetirement(ctx, c.ID, in)
	if e != nil || replay.Retirement.Audit[0].ID != receipt.Retirement.Audit[0].ID {
		t.Fatal("archive retry changed receipt", e)
	}
	for _, act := range []func() error{func() error { return svc.Ingest(ctx, c.ID, []model.Point{{Time: now, Good: 90, Total: 100}}) }, func() error { return svc.Evaluate(ctx, c.ID) }, func() error {
		d := model.DefinitionFromSLO(c)
		_, e := svc.ApplyDefinition(ctx, model.Definition{SLO: &d}, 2)
		return e
	}} {
		if e = act(); e == nil {
			t.Fatal("archived SLO accepted a write")
		}
	}
	frozen, e := st.Read(ctx, c.ID)
	if e != nil || len(frozen.Points) != 1 || frozen.Alert.Phase != "archived" {
		t.Fatal(frozen, e)
	}
	if _, e = svc.SLORevision(ctx, c.ID, 1); e != nil {
		t.Fatal("retained revision missing", e)
	}
	revision.Manifest.Spec.SLOs = []v1.JourneySLO{{ID: c.ID}}
	revision.Digest = "new-reference"
	if _, e = js.ApplyJourney(ctx, revision, 2); e == nil {
		t.Fatal("archived resource gained a new journey reference")
	}
	if url != "" {
		reopened, e := store.NewPostgres(ctx, url)
		if e != nil {
			t.Fatal(e)
		}
		defer reopened.Close()
		st = reopened
		svc = service.New(st, nil)
		svc.Now = func() time.Time { return now }
		recovered, e := svc.RetirementReceipt(ctx, c.ID)
		if e != nil || recovered.Retirement.Audit[0].Actor != "local_user:retirement-engineer" {
			t.Fatal("restart lost receipt", recovered, e)
		}
	}
	deletion, e := svc.PlanRetirement(ctx, c.ID, "delete")
	if e != nil {
		t.Fatal(e)
	}
	deleteIn := v1.RetirementInput{Action: "delete", ExpectedRevision: 2, Digest: deletion.Digest}
	if _, e = svc.ApplyRetirement(ctx, c.ID, deleteIn); e == nil {
		t.Fatal("irreversible deletion lacked confirmation")
	}
	deleteIn.ConfirmName = c.ID
	deleted, e := svc.ApplyRetirement(ctx, c.ID, deleteIn)
	if e != nil || deleted.DeletedAt == nil {
		t.Fatal(deleted, e)
	}
	if _, e = st.Read(ctx, c.ID); !errors.Is(e, model.ErrNotFound) {
		t.Fatal("definition retained after purge", e)
	}
	if e = svc.Create(ctx, c); !errors.Is(e, model.ErrConflict) {
		t.Fatal("deleted identifier recycled", e)
	}
	audit, e := svc.RetirementReceipt(ctx, c.ID)
	if e != nil || audit.DeletedAt == nil || len(audit.Retirement.Audit) != 2 {
		t.Fatal("deletion lost audit", audit, e)
	}
	if _, e = svc.ApplyRetirement(ctx, c.ID, deleteIn); e != nil {
		t.Fatal("delete retry failed", e)
	}
}
func TestConcurrentRetirementCannotGainReference(t *testing.T) {
	for i := 0; i < 20; i++ {
		ctx := context.Background()
		st := store.NewMemory()
		svc := service.New(st, nil)
		c := model.SLO{ID: "race", Project: "p", Service: "api", Name: "API", Target: .99, WindowSeconds: 60, StepSeconds: 10}
		if e := svc.Create(ctx, c); e != nil {
			t.Fatal(e)
		}
		plan, e := svc.PlanRetirement(ctx, c.ID, "archive")
		if e != nil {
			t.Fatal(e)
		}
		j := model.JourneyRevision{Manifest: model.Journey{Metadata: model.ResourceMetadata{Name: "race-journey"}, Spec: v1.JourneySpec{SLOs: []v1.JourneySLO{{ID: c.ID, Required: true}}}}, Digest: "race"}
		var a, b error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, a = svc.ApplyRetirement(ctx, c.ID, v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: plan.Digest})
		}()
		go func() { defer wg.Done(); _, b = st.ApplyJourney(ctx, j, 0) }()
		wg.Wait()
		if a == nil && b == nil {
			t.Fatal("archive and new reference both committed")
		}
	}
}

func TestRetirementCleanupFailureSurvivesRestartAndCredentialRotation(t *testing.T) {
	t.Run("memory", func(t *testing.T) { retirementFailure(t, store.NewMemory(), "") })
	t.Run("postgres", func(t *testing.T) { st, url := postgres(t); retirementFailure(t, st, url) })
}
func retirementFailure(t *testing.T, st store.Store, url string) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc := service.New(st, nil)
	svc.Now = func() time.Time { return now }
	currentKey := "old-fixture-key"
	expectedKey := currentKey
	available := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available {
			http.Error(w, "private-provider-error", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+expectedKey {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/-/reload" {
			w.WriteHeader(200)
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
	}))
	defer backend.Close()
	target := rules.Target{Kind: "prometheus", Connection: "primary", Namespace: "owned", Directory: t.TempDir(), QueryURL: backend.URL, Reload: true}
	manager := &rules.Manager{Tenant: "default", DynamicTarget: func(string) (rules.Target, bool) { fresh := target; fresh.BearerToken = currentKey; return fresh, true }}
	svc.RuleManager = manager
	manager.PublicationGuard = svc.PinRuleDestination
	c := model.SLO{ID: "retry-cleanup", Source: "primary", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 10, WindowSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if e := svc.Create(ctx, c); e != nil {
		t.Fatal(e)
	}
	stored, _ := st.Read(ctx, c.ID)
	plan, e := manager.Plan(stored.SLO)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = manager.Apply(ctx, stored.SLO, plan.Digest); e != nil {
		t.Fatal(e)
	}
	retirement, e := svc.PlanRetirement(ctx, c.ID, "archive")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ApplyRetirement(ctx, c.ID, v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: retirement.Digest}); e != nil {
		t.Fatal(e)
	}
	now = now.Add(31 * time.Second)
	receipt, e := svc.ReconcileRetirement(ctx, c.ID)
	if e == nil || receipt.Retirement.Cleanup != "failed" || receipt.Retirement.Attempts != 1 {
		t.Fatal("outage did not retain retry", receipt, e)
	}
	if url != "" {
		reopened, e := store.NewPostgres(ctx, url)
		if e != nil {
			t.Fatal(e)
		}
		defer reopened.Close()
		st = reopened
	}
	recovered := service.New(st, nil)
	recovered.RuleManager = manager
	recovered.Now = func() time.Time { return now }
	expectedKey = "rotated-fixture-key"
	available = true
	now = now.Add(time.Minute)
	receipt, e = recovered.ReconcileRetirement(ctx, c.ID)
	if e == nil || receipt.Retirement.Cleanup != "failed" {
		t.Fatal("stale credentials passed", receipt, e)
	}
	currentKey = expectedKey
	now = now.Add(time.Minute)
	receipt, e = recovered.ReconcileRetirement(ctx, c.ID)
	if e != nil || receipt.Retirement.Cleanup != "complete" || receipt.Retirement.Attempts != 3 {
		t.Fatal("rotation did not recover cleanup", receipt, e)
	}
	raw, _ := json.Marshal(receipt)
	for _, secret := range []string{currentKey, "old-fixture-key", "private-provider-error"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("receipt leaked provider data")
		}
	}
}
