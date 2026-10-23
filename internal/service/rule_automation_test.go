package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
)

func autoFixture(t *testing.T) (*service.Service, *rules.Manager, *atomic.Int64, *atomic.Int64, *time.Time) {
	t.Helper()
	var reloads, code atomic.Int64
	code.Store(200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/reload" {
			reloads.Add(1)
			w.WriteHeader(int(code.Load()))
			return
		}
		http.Error(w, "private-provider-body", 500)
	}))
	t.Cleanup(backend.Close)
	svc := service.New(store.NewMemory(), nil)
	now := time.Now().UTC()
	svc.Now = func() time.Time { return now }
	svc.TenantID = "alpha"
	slo := model.SLO{ID: "api", Project: "test", Name: "API", Service: "api", Source: "primary", Target: .99, WindowSeconds: 3600, StepSeconds: 60, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if e := svc.Create(context.Background(), slo); e != nil {
		t.Fatal(e)
	}
	m := &rules.Manager{Tenant: "alpha", Targets: map[string]rules.Target{"primary": {Connection: "primary", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir(), QueryURL: backend.URL, Reload: true, AutoDeploy: true}}}
	svc.RuleManager = m
	return svc, m, &reloads, &code, &now
}
func TestAutomaticRulesCreateEditRetryPauseAndRestart(t *testing.T) {
	s, m, reloads, code, now := autoFixture(t)
	ctx := context.Background()
	state, e := s.ReconcileRules(ctx, "api", m)
	if e != nil || state.State != "configured" || state.Revision != 1 || reloads.Load() != 1 {
		t.Fatal(state, e, reloads.Load())
	}
	if _, e = s.ReconcileRules(ctx, "api", m); e != nil || reloads.Load() != 1 {
		t.Fatal("immediate duplicate deployment", e, reloads.Load())
	}
	old, _ := s.Store.Read(ctx, "api")
	d := model.DefinitionFromSLO(old.SLO)
	d.Spec.Target = .999
	d.Spec.GoodQuery = "vector(98)"
	definition := model.Definition{SLO: &d}
	reviewed, err := s.PlanDefinition(ctx, definition)
	if err != nil || !reviewed.MeasurementChanged || len(reviewed.Warnings) < 2 || !strings.Contains(strings.Join(reviewed.Warnings, " "), "automatic") {
		t.Fatal("measurement edits must retain automatic deployment disclosure", reviewed, err)
	}
	if _, e = s.ApplyDefinition(ctx, model.Definition{SLO: &d}, 1); e != nil {
		t.Fatal(e)
	}
	state, e = s.ReconcileRules(ctx, "api", m)
	if e != nil || state.Revision != 2 || reloads.Load() != 2 {
		t.Fatal(state, e)
	}
	plan, _ := m.Plan(d.SLO())
	files, _ := filepath.Glob(filepath.Join(plan.Target.Directory, "*.yaml"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	raw, _ := os.ReadFile(files[0])
	var file rules.File
	if yaml.Unmarshal(raw, &file) != nil {
		t.Fatal("invalid rules")
	}
	for _, rule := range file.Groups[0].Rules {
		if rule.Record == model.RuleOutputMetric && rule.Labels["nines_revision"] != "2" {
			t.Fatal("old revision deployed", rule)
		}
	}
	if _, e = s.SetRuleAutomation(ctx, "api", 1, true, m); !errors.Is(e, model.ErrRevisionConflict) {
		t.Fatal("stale pause accepted", e)
	}
	if state, e = s.SetRuleAutomation(ctx, "api", 2, true, m); e != nil || !state.Paused {
		t.Fatal(state, e)
	}
	restarted := service.New(s.Store, nil)
	restarted.TenantID = s.TenantID
	restarted.Now = s.Now
	*now = now.Add(2 * time.Minute)
	state, e = restarted.ReconcileRules(ctx, "api", m)
	if e != nil || !state.Paused || reloads.Load() != 2 {
		t.Fatal("pause lost on restart", state, e)
	}
	if _, e = restarted.SetRuleAutomation(ctx, "api", 2, false, m); e != nil {
		t.Fatal(e)
	}
	code.Store(404)
	state, e = restarted.ReconcileRules(ctx, "api", m)
	if e != nil || state.State != "failed" || state.Code != "prometheus_lifecycle_unavailable" {
		t.Fatal("reload failure claimed success", state, e)
	}
	count := reloads.Load()
	if _, e = restarted.ReconcileRules(ctx, "api", m); e != nil || reloads.Load() != count {
		t.Fatal("failure backoff ignored", e)
	}
	body, _ := json.Marshal(state)
	if strings.Contains(string(body), "private-provider") {
		t.Fatal("private error leaked")
	}
	*now = now.Add(6 * time.Second)
	code.Store(200)
	state, e = restarted.ReconcileRules(ctx, "api", m)
	if e != nil || state.State != "configured" || reloads.Load() != count+1 {
		t.Fatal("retry did not recover", state, e)
	}
}
func TestAutomaticRulesAreOptInAndRejectForeignFiles(t *testing.T) {
	s, m, reloads, _, _ := autoFixture(t)
	ctx := context.Background()
	target := m.Targets["primary"]
	target.AutoDeploy = false
	m.Targets["primary"] = target
	if state, e := s.ReconcileRules(ctx, "api", m); e != nil || state.State != "manual" || reloads.Load() != 0 {
		t.Fatal(state, e)
	}
	files, _ := filepath.Glob(filepath.Join(target.Directory, "*.yaml"))
	if len(files) != 0 {
		t.Fatal("manual policy wrote files")
	}
	target.AutoDeploy = true
	m.Targets["primary"] = target
	record, _ := s.Store.Read(ctx, "api")
	plan, _ := m.Plan(record.SLO)
	for i := range plan.Group.Rules {
		plan.Group.Rules[i].Labels["nines_tenant"] = "other"
	}
	foreign, _ := yaml.Marshal(rules.File{Groups: []rules.Group{plan.Group}})
	path := filepath.Join(target.Directory, target.Namespace+"-"+plan.Group.Name+".yaml")
	if e := os.WriteFile(path, foreign, 0600); e != nil {
		t.Fatal(e)
	}
	state, e := s.ReconcileRules(ctx, "api", m)
	if e == nil || state.State != "failed" || reloads.Load() != 0 {
		t.Fatal("foreign rules overwritten", state, e)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(foreign) {
		t.Fatal("foreign content modified")
	}
}
