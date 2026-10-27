package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
)

func TestRealPrometheusLifecycleAndMinEvents(t *testing.T) {
	base, directory := os.Getenv("PROMETHEUS_TEST_URL"), os.Getenv("PROMETHEUS_TEST_RULE_DIRECTORY")
	if base == "" || directory == "" {
		t.Skip("use make test-prometheus for the disposable loopback fixture")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("use a private absolute rule directory")
	}
	realLifecycleAndMinEvents(t, "prometheus", base, directory)
}
func TestRealMimirLifecycleAndMinEvents(t *testing.T) {
	base := os.Getenv("MIMIR_TEST_URL")
	if base == "" {
		t.Skip("use make test-mimir for the disposable loopback fixture")
	}
	realLifecycleAndMinEvents(t, "mimir", base, "")
}

// Exercise the compiled expressions and replacement of alert rules in actual
// evaluators, including revision labels and disappearance of an active alert.
func realLifecycleAndMinEvents(t *testing.T, kind, base, directory string) {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("use a disposable loopback origin")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		t.Fatal("refusing non-loopback rule writes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	readyPath, queryURL, org := "/-/ready", base, ""
	if kind == "mimir" {
		readyPath, queryURL, org = "/ready", base+"/prometheus", "nines-lifecycle-test"
	}
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+readyPath, nil)
		resp, e := client.Do(req)
		ready := e == nil && resp.StatusCode == 200
		if resp != nil {
			_ = resp.Body.Close()
		}
		if ready {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("backend did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	s := fixture()
	s.ID = "lifecycle-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	s.GoodQuery, s.TotalQuery = "vector(0)", "vector(100)"
	s.StepSeconds, s.WindowSeconds = 1, 8
	s.Policy = &model.Policy{ShortWindowSeconds: 2, LongWindowSeconds: 4, BurnRate: 2, ForSeconds: 1, MinCoverage: 1, MinEvents: 300}
	target := Target{Kind: kind, Connection: "production", QueryURL: queryURL, RulerURL: queryURL, MimirOrgID: org, Namespace: "nines-lifecycle", Directory: directory, Reload: kind == "prometheus"}
	manager := Manager{Tenant: "lifecycle", Targets: map[string]Target{"production": target}}
	var collector *prometheus.Client
	if kind == "mimir" {
		collector, err = prometheus.NewMimir(queryURL, org, "")
	} else {
		collector, err = prometheus.New(queryURL, "")
	}
	if err != nil {
		t.Fatal(err)
	}
	queryCount := func(expression string) (int, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, queryURL+"/api/v1/query?query="+url.QueryEscape(expression), nil)
		if e != nil {
			return 0, e
		}
		if org != "" {
			req.Header.Set("X-Scope-OrgID", org)
		}
		resp, e := client.Do(req)
		if e != nil {
			return 0, e
		}
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Status string
			Data   struct{ Result []json.RawMessage }
			Error  string
		}
		if e = json.NewDecoder(resp.Body).Decode(&body); e != nil {
			return 0, e
		}
		if resp.StatusCode != 200 || body.Status != "success" {
			return 0, fmt.Errorf("query failed: HTTP %d %s", resp.StatusCode, body.Error)
		}
		return len(body.Data.Result), nil
	}
	for i, stage := range []struct {
		lifecycle  v1.SLOLifecycle
		minEvents  float64
		wantFiring bool
	}{
		{v1.LifecycleAspirational, 300, false},
		{v1.LifecycleCommitted, 300, false}, // Short window has 200 events, long window 400.
		{v1.LifecycleCommitted, 150, true},
		{v1.LifecycleAspirational, 150, false},
	} {
		s.Revision, s.Lifecycle, s.Policy.MinEvents = int64(i+1), stage.lifecycle, stage.minEvents
		plan, e := manager.Plan(s)
		if e != nil {
			t.Fatal(e)
		}
		alerts := 0
		for _, rule := range plan.Group.Rules {
			if rule.Alert != "" {
				alerts++
			}
		}
		if (alerts == 1) != s.AlertsEnabled() {
			t.Fatal("compiled lifecycle mismatch", alerts)
		}
		result, e := manager.Deploy(ctx, s, plan.Digest)
		if e != nil || result.Activation == nil || result.Activation.State != "accepted" {
			t.Fatal("rule activation failed", result, e)
		}
		earliest := time.Now().Add(7 * time.Second)
		var stableSince time.Time
		for {
			_, evaluation := manager.Inspect(ctx, s, time.Now().UTC())
			output := collector.RuleOutput(ctx, OutputIdentity(s, manager.Tenant, ""), time.Now().UTC())
			firing, queryErr := queryCount(fmt.Sprintf(`ALERTS{nines_slo=%q,alertstate="firing"}`, s.ID))
			coverage, coverageErr := queryCount(fmt.Sprintf(`nines:slo_coverage:ratio{nines_slo=%q,nines_revision=%q,window="4s"} == 1`, s.ID, fmt.Sprint(s.Revision)))
			if time.Now().After(earliest) && evaluation.State == "evaluating" && output.State == "verified" && queryErr == nil && coverageErr == nil && coverage == 1 {
				// Loaded rules and full coverage can precede completion of `for`.
				// Observe the requested phase continuously instead of failing on that
				// legitimate intermediate state or accepting one inactive sample.
				if (firing == 1) == stage.wantFiring {
					if stableSince.IsZero() {
						stableSince = time.Now()
					}
					if time.Since(stableSince) >= 2*time.Second {
						break
					}
				} else {
					stableSince = time.Time{}
				}
			} else {
				stableSince = time.Time{}
			}
			if ctx.Err() != nil {
				t.Fatalf("%s stage %d did not reach expected firing=%v: %s/%s, query=%v coverage=%v", kind, i, stage.wantFiring, evaluation.State, output.State, queryErr, coverageErr)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	t.Log("Aspirational rules retained measurements without alerts; committed rules gated low short-window traffic; lowering the count enabled firing; demotion removed the active external alert.")
}
