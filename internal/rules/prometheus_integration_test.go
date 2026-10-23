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

	"github.com/ishantanu/nines/internal/prometheus"
)

func TestRealPrometheusReviewedActivation(t *testing.T) {
	base, directory := os.Getenv("PROMETHEUS_TEST_URL"), os.Getenv("PROMETHEUS_TEST_RULE_DIRECTORY")
	if base == "" || directory == "" {
		t.Skip("use make test-prometheus for the disposable shared-mount fixture")
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || !filepath.IsAbs(directory) {
		t.Fatal("use the isolated loopback fixture")
	}
	ip := net.ParseIP(endpoint.Hostname())
	if ip == nil || !ip.IsLoopback() {
		t.Fatal("refusing non-loopback activation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := http.Client{Timeout: time.Second}
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/-/ready", nil)
		response, readyErr := client.Do(request)
		ready := readyErr == nil && response.StatusCode == 200
		if response != nil {
			_ = response.Body.Close()
		}
		if ready {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("fixture not ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	slo := fixture()
	slo.GoodQuery = "vector(99)"
	slo.TotalQuery = "vector(100)"
	slo.StepSeconds = 1
	slo.WindowSeconds = 60
	slo.Policy.ShortWindowSeconds = 2
	slo.Policy.LongWindowSeconds = 4
	slo.Policy.ForSeconds = 2
	slo.Policy.BurnRate = 2
	target := Target{Kind: "prometheus", Connection: "production", QueryURL: base, Directory: directory, Namespace: "nines"}
	manager := Manager{Tenant: "acceptance", Targets: map[string]Target{"production": target}}
	plan, err := manager.Plan(slo)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Deploy(ctx, slo, plan.Digest)
	if err != nil || result.State != "written" || result.Activation.State != "manual" || result.Evaluation.State != "not_loaded" {
		t.Fatal("a written file was mistaken for activation", result, err)
	}
	target.Reload = true
	manager.Targets["production"] = target
	if _, err := manager.Deploy(ctx, slo, plan.Digest); err == nil {
		t.Fatal("activation policy changed without a reviewed digest")
	}
	plan, err = manager.Plan(slo)
	if err != nil {
		t.Fatal(err)
	}
	result, err = manager.Deploy(ctx, slo, plan.Digest)
	if err != nil || result.Activation.State != "accepted" {
		t.Fatal("native reload failed", result, err)
	}
	query, err := prometheus.New(base, "")
	if err != nil {
		t.Fatal(err)
	}
	for {
		stored, evaluating := manager.Inspect(ctx, slo, time.Now().UTC())
		output := query.RuleOutput(ctx, OutputIdentity(slo, manager.Tenant, ""), time.Now().UTC())
		if stored.State == "written" && evaluating.State == "evaluating" && output.State == "verified" {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("activation did not reach current runtime and output", stored, evaluating, output)
		}
		time.Sleep(200 * time.Millisecond)
	}
	result, err = manager.Deploy(ctx, slo, plan.Digest)
	if err != nil || result.State != "unchanged" || result.Activation.Code != "rules_already_evaluating" || result.Evaluation.State != "evaluating" {
		t.Fatal("active plan not idempotent", result, err)
	}
	t.Log("File written alone remained not_loaded; reviewed native reload activated exact rules; current output marker verified; repeat apply skipped reload.")
}
