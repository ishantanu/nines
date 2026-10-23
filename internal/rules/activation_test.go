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
	"sync/atomic"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func TestPrometheusReviewedActivation(t *testing.T) {
	for _, test := range []struct {
		name    string
		reload  bool
		code    int
		current bool
		want    string
	}{
		{"manual", false, 200, false, "manual"},
		{"reload", true, 200, true, "accepted"},
		{"forbidden", true, 403, false, "failed"},
		{"lifecycle-disabled", true, 404, false, "failed"},
		{"server-error", true, 500, false, "failed"},
		{"ambiguous-but-current", true, 500, true, "verified"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var plan Plan
			var posts atomic.Int64
			var loaded atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer private-reload-token" {
					t.Error("wrong pinned credentials")
				}
				switch r.URL.Path {
				case "/prefix/-/reload":
					if r.Method != http.MethodPost {
						t.Error("reload must be POST")
					}
					if _, err := os.Stat(filepath.Join(plan.Target.Directory, "nines-"+plan.Group.Name+".yaml")); err != nil {
						t.Error("reload before atomic file write", err)
					}
					posts.Add(1)
					loaded.Store(test.current)
					http.Error(w, "private-provider-body", test.code)
				case "/prefix/api/v1/rules":
					groups := []runtimeGroup{}
					if loaded.Load() {
						groups = append(groups, evaluatedGroup(plan, time.Now().UTC()))
					}
					// Missing lifecycle/access stays separate from runtime evidence. Use a
					// complete current-but-pending group to avoid waiting for a missing group.
					if !test.current && test.reload {
						g := evaluatedGroup(plan, time.Time{})
						groups = append(groups, g)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"groups": groups}})
				default:
					t.Error("unexpected endpoint", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Kind: "prometheus", Connection: "production", QueryURL: server.URL + "/prefix", Directory: t.TempDir(), Namespace: "nines", Reload: test.reload, BearerToken: "private-reload-token"}}}
			var err error
			plan, err = manager.Plan(fixture())
			if err != nil {
				t.Fatal(err)
			}
			result, err := manager.Deploy(context.Background(), fixture(), plan.Digest)
			if err != nil || result.State != "written" || result.Activation == nil || result.Activation.State != test.want || result.Evaluation == nil {
				t.Fatal(result, err)
			}
			wantPosts := int64(0)
			if test.reload {
				wantPosts = 1
			}
			if posts.Load() != wantPosts {
				t.Fatal("unexpected retry", posts.Load())
			}
			if test.current && result.Evaluation.State != "evaluating" {
				t.Fatal(result.Evaluation)
			}
			body, _ := json.Marshal(result)
			if strings.Contains(string(body), "private-") {
				t.Fatal("credential/provider body leaked")
			}
			deployment, _ := manager.Inspect(context.Background(), fixture(), time.Now())
			if deployment.State != "written" {
				t.Fatal("failed reload must retain truthful file evidence", deployment)
			}
			if test.current {
				result, err = manager.Deploy(context.Background(), fixture(), plan.Digest)
				if err != nil || result.State != "unchanged" || result.Activation.Code != "rules_already_evaluating" || posts.Load() != 1 {
					t.Fatal("already active group reloaded", result, err, posts.Load())
				}
			}
		})
	}
}

func TestReloadRefusesRedirectAndUntrustedURLShape(t *testing.T) {
	var redirected atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	code, err := reloadPrometheus(context.Background(), Target{QueryURL: origin.URL, BearerToken: "private"})
	if err != nil || code != http.StatusTemporaryRedirect || redirected.Load() != 0 {
		t.Fatal(code, err, redirected.Load())
	}
	for _, endpoint := range []string{"file:///tmp/rules", "https://user:password@example.test", origin.URL + "?redirect=1", origin.URL + "#fragment"} {
		if _, err := reloadPrometheus(context.Background(), Target{QueryURL: endpoint}); err == nil {
			t.Fatal("invalid reload endpoint accepted", endpoint)
		}
	}
	basic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "rules" || password != "private" || r.URL.Path != "/prefix/-/reload" {
			t.Error("basic auth/path mismatch")
		}
	}))
	defer basic.Close()
	if code, err := reloadPrometheus(context.Background(), Target{QueryURL: basic.URL + "/prefix/", BasicUsername: "rules", BasicPassword: "private"}); err != nil || code != 200 {
		t.Fatal(code, err)
	}
}

func TestReloadHonorsFinalAuthorityAndDigest(t *testing.T) {
	var posts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1) }))
	defer server.Close()
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Kind: "prometheus", Connection: "production", QueryURL: server.URL, Directory: t.TempDir(), Namespace: "nines", Reload: true}}}
	plan, _ := manager.Plan(fixture())
	calls := 0
	manager.BeforeWrite = func() error {
		calls++
		if calls >= 3 {
			return errors.New("lease revoked")
		}
		return nil
	}
	result, err := manager.Deploy(context.Background(), fixture(), plan.Digest)
	if err != nil || result.State != "written" || result.Activation.Code != "reload_not_authorized" || posts.Load() != 0 {
		t.Fatal(result, err, posts.Load(), calls)
	}
	target := manager.Targets["production"]
	target.Reload = false
	manager.Targets["production"] = target
	if _, err := manager.Deploy(context.Background(), fixture(), plan.Digest); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("activation mode changed without review", err)
	}
}
