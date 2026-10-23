package rules

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPrometheusWritesRespectOwnershipAndAuthorizationFence(t *testing.T) {
	s := fixture()
	targets := map[string]Target{"production": {Connection: "production", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir()}}
	manager := Manager{Tenant: "commerce", Targets: targets, OwnerUID: "operator-one"}
	plan, err := manager.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(plan.Target.Directory, "nines-"+plan.Group.Name+".yaml")
	denied := errors.New("lease revoked")
	manager.BeforeWrite = func() error { return denied }
	if _, err := manager.Apply(context.Background(), s, plan.Digest); !errors.Is(err, denied) {
		t.Fatal("authorization fence ignored", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked writer created rule file", err)
	}
	manager.BeforeWrite = nil
	if _, err := manager.Apply(context.Background(), s, plan.Digest); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(file)
	for _, uid := range []string{"", "operator-two"} {
		other := Manager{Tenant: "commerce", Targets: targets, OwnerUID: uid}
		p, _ := other.Plan(s)
		if _, err := other.Apply(context.Background(), s, p.Digest); err == nil {
			t.Fatal("foreign operator ownership replaced", uid)
		}
	}
	after, _ := os.ReadFile(file)
	if string(original) != string(after) {
		t.Fatal("foreign write altered the rule file")
	}
}

func TestPrometheusCancelledApplyDoesNotWrite(t *testing.T) {
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir()}}}
	plan, _ := manager.Plan(fixture())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Apply(ctx, fixture(), plan.Digest); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled apply was not rejected", err)
	}
	files, _ := os.ReadDir(plan.Target.Directory)
	if len(files) != 0 {
		t.Fatal("cancelled apply left rule files")
	}
}

func TestMimirAmbiguousWriteUsesReadbackAndRestartSafeRetry(t *testing.T) {
	for _, test := range []struct {
		name               string
		commit, disconnect bool
	}{
		{"committed-gateway-error", true, false},
		{"not-committed-gateway-error", false, false},
		{"committed-lost-response", true, true},
		{"not-committed-lost-response", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var stored []byte
			writes := 0
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodGet {
					if stored == nil {
						w.WriteHeader(404)
					} else {
						_, _ = w.Write(stored)
					}
					return
				}
				writes++
				body, _ := io.ReadAll(r.Body)
				if test.commit {
					stored = body
				}
				if test.disconnect {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				http.Error(w, "private-provider-error", http.StatusServiceUnavailable)
			}))
			defer backend.Close()
			targets := map[string]Target{"production": {Connection: "production", Kind: "mimir", RulerURL: backend.URL, MimirOrgID: "commerce", Namespace: "nines"}}
			manager := Manager{Tenant: "commerce", Targets: targets}
			plan, _ := manager.Plan(fixture())
			result, err := manager.Apply(context.Background(), fixture(), plan.Digest)
			if !test.commit {
				if err == nil || result.State == "verified" {
					t.Fatal("unconfirmed write reported successful", result, err)
				}
				return
			}
			if err != nil || result.State != "verified" || result.Digest != plan.Digest {
				t.Fatal("committed write not recovered through exact readback", result, err)
			}
			restarted := Manager{Tenant: "commerce", Targets: targets}
			result, err = restarted.Apply(context.Background(), fixture(), plan.Digest)
			if err != nil || result.State != "unchanged" {
				t.Fatal("retry did not read the existing group", result, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if writes != 1 {
				t.Fatal("write was retried without readback", writes)
			}
			var group Group
			if yaml.Unmarshal(stored, &group) != nil || !sameGroup(group, plan.Group) {
				t.Fatal("stored group differs from reviewed plan")
			}
		})
	}
}

func TestMimirCannotSilentlyDiscardBackendMeasurementOptions(t *testing.T) {
	var stored []byte
	writes := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(stored)
			return
		}
		writes++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer backend.Close()
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", RulerURL: backend.URL, MimirOrgID: "commerce", Namespace: "nines"}}}
	plan, err := manager.Plan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := yaml.Marshal(plan.Group)
	stored = append(body, []byte("source_tenants: [another-organization]\n")...)
	if _, err := manager.Apply(context.Background(), fixture(), plan.Digest); err == nil || writes != 0 {
		t.Fatal("unsupported backend options silently overwritten", writes, err)
	}
}

func TestPrometheusRechecksAuthorizationBeforeActivatingFile(t *testing.T) {
	manager := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Directory: t.TempDir(), Namespace: "nines"}}}
	plan, err := manager.Plan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	denied := errors.New("lease expired during preparation")
	manager.BeforeWrite = func() error {
		checks++
		if checks == 2 {
			return denied
		}
		return nil
	}
	if _, err := manager.Apply(context.Background(), fixture(), plan.Digest); !errors.Is(err, denied) || checks != 2 {
		t.Fatal("activation was not fenced", checks, err)
	}
	files, err := os.ReadDir(plan.Target.Directory)
	if err != nil || len(files) != 0 {
		t.Fatal("failed activation left rule or temporary files", files, err)
	}
}
