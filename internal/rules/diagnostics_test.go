package rules

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func evaluatedGroup(p Plan, at time.Time) runtimeGroup {
	seconds, _ := DurationSeconds(p.Group.Interval)
	out := runtimeGroup{Name: p.Group.Name, File: p.Target.Namespace, Interval: float64(seconds)}
	for _, rule := range p.Group.Rules {
		name, kind := rule.Record, "recording"
		if rule.Alert != "" {
			name, kind = rule.Alert, "alerting"
		}
		duration, _ := DurationSeconds(rule.For)
		out.Rules = append(out.Rules, runtimeRule{Name: name, Type: kind, Query: rule.Expr, Labels: rule.Labels, Annotations: rule.Annotations, Duration: float64(duration), Health: "ok", LastEvaluation: at})
	}
	return out
}

func TestMimirRuntimeRequiresExactNamespaceAndCompleteResponse(t *testing.T) {
	for _, test := range []struct{ name, file, token, state, code string }{
		{"current", "nines", "", "evaluating", "rules_evaluating"},
		{"other-namespace", "another-namespace", "", "not_loaded", "ruler_namespace_mismatch"},
		{"namespace-omitted", "", "", "unknown", "ruler_namespace_unverified"},
		{"partial-page", "nines", "more-groups", "unknown", "evaluator_response_incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var group runtimeGroup
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("X-Scope-OrgID") != "commerce" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
					t.Error("incorrect method or tenant/auth scope")
				}
				if strings.Contains(r.URL.Path, "/config/") {
					w.WriteHeader(404)
					return
				}
				if r.URL.Query().Get("file") != "nines" || r.URL.Query().Get("rule_group") != group.Name {
					t.Error("runtime request not scoped to the reviewed namespace/group")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"groups": []runtimeGroup{group}, "groupNextToken": test.token}})
			}))
			defer backend.Close()
			manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", RulerURL: backend.URL, MimirOrgID: "commerce", Namespace: "nines", BearerToken: "fixture-secret"}}}
			plan, _ := manager.Plan(fixture())
			now := time.Now().UTC()
			group = evaluatedGroup(plan, now)
			group.File = test.file
			_, runtime := manager.Inspect(context.Background(), fixture(), now)
			if runtime.State != test.state || runtime.Code != test.code || runtime.NextAction == "" {
				t.Fatal("incorrect namespace/completeness evidence", runtime)
			}
		})
	}
}

func TestInspectionAccessAndProviderFailuresAreActionableAndPrivate(t *testing.T) {
	for _, code := range []int{401, 403, 404, 503} {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Error("inspection mutated provider")
			}
			http.Error(w, "fixture-secret-private-provider-error", code)
		}))
		manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", RulerURL: backend.URL, MimirOrgID: "commerce", Namespace: "nines"}}}
		deployment, runtime := manager.Inspect(context.Background(), fixture(), time.Now())
		backend.Close()
		if deployment.Code == "" || runtime.Code == "" || deployment.NextAction == "" || runtime.NextAction == "" || runtime.State != "unknown" {
			t.Fatal(deployment, runtime)
		}
		body, _ := json.Marshal([]any{deployment, runtime})
		if strings.Contains(string(body), "fixture-secret") || strings.Contains(string(body), backend.URL) {
			t.Fatal("provider data leaked", string(body))
		}
		if code == 401 || code == 403 {
			if deployment.Code != "ruler_access_denied" || runtime.Code != "evaluator_access_denied" {
				t.Fatal(deployment, runtime)
			}
		}
	}
}

func TestRuleFileInspectionRejectsSymlinkAndMultipleDocuments(t *testing.T) {
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir()}}}
	plan, _ := manager.Plan(fixture())
	file := filepath.Join(plan.Target.Directory, "nines-"+plan.Group.Name+".yaml")
	for _, mode := range []string{"multiple-documents", "unsupported-fields", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			if mode != "symlink" {
				body := plan.YAML + "\n---\ngroups: []\n"
				if mode == "unsupported-fields" {
					body = plan.YAML + "\nunsupported: true\n"
				}
				if err := os.WriteFile(file, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file+".real", []byte(plan.YAML), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(file+".real", file); err != nil {
					t.Fatal(err)
				}
			}
			deployment, _ := manager.Inspect(context.Background(), fixture(), time.Now())
			if deployment.State != "blocked" || deployment.Code != "rule_file_unreadable" {
				t.Fatal(deployment)
			}
			if _, err := manager.Apply(context.Background(), fixture(), plan.Digest); err == nil {
				t.Fatal("unsafe file replaced")
			}
		})
	}
}

func TestDestinationChangedAtWriteGuardInvalidatesReviewedDigest(t *testing.T) {
	for _, kind := range []string{"prometheus", "mimir"} {
		t.Run(kind, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("stale reviewed destination was written")
				}
				w.WriteHeader(404)
			}))
			defer backend.Close()
			manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: kind, Directory: t.TempDir(), RulerURL: backend.URL, Namespace: "nines", MimirOrgID: "commerce"}}}
			plan, _ := manager.Plan(fixture())
			manager.BeforeWrite = func() error {
				target := manager.Targets["production"]
				target.Namespace = "changed"
				manager.Targets["production"] = target
				return nil
			}
			if _, err := manager.Apply(context.Background(), fixture(), plan.Digest); !errors.Is(err, model.ErrRevisionConflict) {
				t.Fatal("stale destination not rejected", err)
			}
			files, _ := os.ReadDir(plan.Target.Directory)
			if len(files) != 0 {
				t.Fatal("stale destination left files")
			}
		})
	}
}

func TestRuntimeMissingGroupsDoesNotProveAbsence(t *testing.T) {
	for _, body := range []string{`{"status":"success"}`, `{"status":"success","data":{"groups":null}}`} {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir(), QueryURL: backend.URL}}}
		_, evaluation := manager.Inspect(context.Background(), fixture(), time.Now())
		backend.Close()
		if evaluation.State != "unknown" || evaluation.Code != "evaluator_response_incomplete" {
			t.Fatal("malformed response claimed a missing rule group", evaluation)
		}
	}
}
