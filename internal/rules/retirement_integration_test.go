package rules

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestRealPrometheusRetirement(t *testing.T) {
	base, dir := os.Getenv("PROMETHEUS_TEST_URL"), os.Getenv("PROMETHEUS_TEST_RULE_DIRECTORY")
	if base == "" || dir == "" {
		t.Skip("use make test-prometheus for disposable retirement acceptance")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("private absolute rule directory required")
	}
	realRetirement(t, "prometheus", base, dir)
}
func TestRealMimirRetirement(t *testing.T) {
	base := os.Getenv("MIMIR_TEST_URL")
	if base == "" {
		t.Skip("use make test-mimir for disposable retirement acceptance")
	}
	realRetirement(t, "mimir", base, "")
}
func realRetirement(t *testing.T, kind, base, dir string) {
	t.Helper()
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("disposable loopback origin required")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		t.Fatal("nonloopback writes refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	ready := base + "/-/ready"
	target := Target{Kind: kind, Connection: "production", Namespace: "retirement-proof", Directory: dir, QueryURL: base, Reload: kind == "prometheus"}
	if kind == "mimir" {
		ready = base + "/ready"
		target.QueryURL = base + "/prometheus"
		target.RulerURL = target.QueryURL
		target.MimirOrgID = "retirement-proof"
	}
	httpClient := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	wait := func(label string, check func() bool) {
		t.Helper()
		for {
			if check() {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("retirement acceptance timeout", label)
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	wait("backend ready", func() bool {
		r, _ := http.NewRequestWithContext(ctx, "GET", ready, nil)
		res, e := httpClient.Do(r)
		if res != nil {
			_ = res.Body.Close()
		}
		return e == nil && res.StatusCode == 200
	})
	slo := fixture()
	slo.ID = "retirement-proof"
	slo.Revision = 1
	slo.GoodQuery = "vector(99)"
	slo.TotalQuery = "vector(100)"
	slo.StepSeconds = 1
	slo.WindowSeconds = 60
	slo.Policy = nil
	slo.Policies = nil
	manager := Manager{Tenant: "retirement-proof", Targets: map[string]Target{"production": target}}
	plan, e := manager.Plan(slo)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = manager.Deploy(ctx, slo, plan.Digest); e != nil {
		t.Fatal(e)
	}
	wait("group loaded", func() bool {
		stage := InspectEvaluation(ctx, slo, plan.Group, target, time.Now().UTC())
		return stage.State == "evaluating"
	})
	slo.Retirement = &v1.SLORetirement{ArchivedAt: time.Now().UTC(), OriginalRevision: model.ConfigRevision(slo), RouteDigest: RouteDigest(target), Group: plan.Group.Name, Cleanup: "pending"}
	slo.Revision = 2
	wait("owned config and runtime removed", func() bool {
		done, e := manager.Retire(ctx, slo)
		if e != nil {
			t.Fatal(e)
		}
		return done
	})
	if kind == "prometheus" {
		if _, e = os.Lstat(filepath.Join(dir, target.Namespace+"-"+plan.Group.Name+".yaml")); !os.IsNotExist(e) {
			t.Fatal("owned file remains", e)
		}
	} else {
		code, _, e := rulerRequest(ctx, target, http.MethodGet, "/config/v1/rules/"+target.Namespace+"/"+plan.Group.Name, nil)
		if e != nil || code != 404 {
			t.Fatal("ruler config remains", code, e)
		}
	}
	t.Log("Verified owned removal and runtime absence in", kind)
}
