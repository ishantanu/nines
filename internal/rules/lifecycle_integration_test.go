package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
)

// The SLO group is deployed once. Only a separate synthetic input group changes;
// pending timers and independent recovery therefore belong to the same revision.
func TestRealMimirIndependentAlertLifecycle(t *testing.T) {
	base := strings.TrimRight(os.Getenv("MIMIR_TEST_URL"), "/")
	if base == "" {
		t.Skip("run make test-mimir with its disposable loopback fixture")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("use a disposable loopback origin")
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("refusing non-loopback writes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	target := Target{Connection: "production", Kind: "mimir", QueryURL: base + "/prometheus", RulerURL: base + "/prometheus", MimirOrgID: "nines-policy-acceptance", Namespace: fmt.Sprintf("policy-%d", time.Now().UnixNano())}
	sourceGroup := Group{Name: "synthetic-input", Interval: "1s"}
	labels := map[string]string{"run": target.Namespace}
	setGood := func(good float64) {
		t.Helper()
		sourceGroup.Rules = []Rule{{Record: "fixture:nines_good:interval", Expr: fmt.Sprintf("vector(%g)", good), Labels: labels}, {Record: "fixture:nines_total:interval", Expr: "vector(100)", Labels: labels}}
		body, err := yaml.Marshal(sourceGroup)
		if err != nil {
			t.Fatal(err)
		}
		code, _, err := rulerRequest(ctx, target, http.MethodPost, "/config/v1/rules/"+target.Namespace, body)
		if err != nil || code != http.StatusAccepted {
			t.Fatal("synthetic input update", code, err)
		}
		t.Logf("Synthetic source now requests %g/100 good events; SLO definition and revision remain unchanged", good)
	}
	// Readiness uses this fixture only; the runner may start packages concurrently.
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/ready", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("disposable Mimir readiness timed out")
		case <-time.After(time.Second):
		}
	}
	setGood(100)
	slo := fixture()
	slo.ID = "independent-policies"
	slo.Revision = 3
	// Give the distributed ruler/query/ingestion path a completed five-second
	// interval. At a one-second cadence, transient visibility gaps reset strict
	// full-coverage alerts and make pending/recovery timing nondeterministic.
	// Keep MinCoverage=1: this fixture must never relax the production guard.
	slo.StepSeconds = 5
	slo.WindowSeconds = 60
	slo.GoodQuery = fmt.Sprintf(`sum(fixture:nines_good:interval{run=%q})`, target.Namespace)
	slo.TotalQuery = fmt.Sprintf(`sum(fixture:nines_total:interval{run=%q})`, target.Namespace)
	slo.Policy = nil
	slo.Policies = []model.Policy{
		{Name: "critical", Severity: "critical", ShortWindowSeconds: 10, LongWindowSeconds: 20, BurnRate: 8, ForSeconds: 10, MinCoverage: 1},
		{Name: "warning", Severity: "warning", ShortWindowSeconds: 20, LongWindowSeconds: 40, BurnRate: 2, ForSeconds: 30, MinCoverage: 1},
	}
	manager := Manager{Tenant: "acceptance", Targets: map[string]Target{"production": target}}
	plan, err := manager.Plan(slo)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := manager.Apply(ctx, slo, plan.Digest); err != nil || result.State != "accepted" {
		t.Fatal(result, err)
	}
	type liveRule struct {
		Health         string            `json:"health"`
		LastError      string            `json:"lastError"`
		Query          string            `json:"query"`
		State          string            `json:"state"`
		Labels         map[string]string `json:"labels"`
		Duration       float64           `json:"duration"`
		LastEvaluation time.Time         `json:"lastEvaluation"`
		Alerts         []struct {
			ActiveAt time.Time         `json:"activeAt"`
			Labels   map[string]string `json:"labels"`
		} `json:"alerts"`
	}
	last := map[string]string{}
	pending := map[string]time.Time{}
	fired := map[string]bool{}
	read := func() map[string]liveRule {
		t.Helper()
		path := "/api/v1/rules?file=" + url.QueryEscape(target.Namespace) + "&rule_group=" + url.QueryEscape(plan.Group.Name)
		code, body, err := rulerRequest(ctx, target, http.MethodGet, path, nil)
		if err != nil || code != http.StatusOK {
			t.Fatal("runtime read failed", code, err)
		}
		var response struct {
			Status string `json:"status"`
			Data   struct {
				Groups []struct {
					Name  string     `json:"name"`
					File  string     `json:"file"`
					Rules []liveRule `json:"rules"`
				} `json:"groups"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil || response.Status != "success" {
			t.Fatal("invalid runtime envelope", err)
		}
		out := map[string]liveRule{}
		for _, group := range response.Data.Groups {
			if group.Name != plan.Group.Name || group.File != target.Namespace {
				t.Fatal("runtime escaped requested group or namespace")
			}
			for _, rule := range group.Rules {
				if rule.LastError != "" || rule.Health == "err" {
					t.Fatalf("fixture rule evaluation failed: %s", rule.LastError)
				}
				policy := rule.Labels["nines_policy"]
				if policy == "" {
					continue
				}
				if rule.Labels["nines_revision"] != "3" || rule.Labels["severity"] != policy {
					t.Fatal("wrong alert revision or severity", rule.Labels)
				}
				out[policy] = rule
				if last[policy] != rule.State {
					t.Logf("revision=3 policy=%s state=%s evaluated=%s", policy, rule.State, rule.LastEvaluation.Format(time.RFC3339Nano))
					last[policy] = rule.State
				}
				if rule.State == "pending" {
					if len(rule.Alerts) != 1 || rule.Alerts[0].ActiveAt.IsZero() {
						t.Fatal("pending alert missing activeAt")
					}
					pending[policy] = rule.Alerts[0].ActiveAt
				}
				if rule.State == "firing" {
					if pending[policy].IsZero() || len(rule.Alerts) != 1 || !pending[policy].Equal(rule.Alerts[0].ActiveAt) {
						t.Fatal("alert fired without observed continuous pending state", policy)
					}
					if rule.LastEvaluation.Sub(pending[policy]) < time.Duration(rule.Duration*float64(time.Second))-100*time.Millisecond {
						t.Fatal("alert fired before its configured pending duration", policy)
					}
					fired[policy] = true
				}
			}
		}
		return out
	}
	wait := func(phase string, condition func(map[string]liveRule) bool) {
		t.Helper()
		// Each phase needs its own window. Startup and earlier pending timers must
		// not consume the recovery window that this phase is required to prove.
		phaseCtx, stop := context.WithTimeout(ctx, 3*time.Minute)
		defer stop()
		checks := 0
		for {
			checks++
			if strings.HasPrefix(phase, "critical recovers") && checks == 50 {
				for _, metric := range []string{"nines:slo_burn_rate:ratio", "nines:slo_coverage:ratio", "nines:sli_good:interval", "nines:sli_total:interval", "fixture:nines_good:interval"} {
					_, body, err := rulerRequest(ctx, target, http.MethodGet, "/api/v1/query?query="+url.QueryEscape(metric), nil)
					t.Logf("fixture diagnostics %s: %s (%v)", metric, body, err)
				}
			}
			if condition(read()) {
				t.Log("Verified phase:", phase)
				return
			}
			select {
			case <-phaseCtx.Done():
				t.Fatal("alert lifecycle timed out in phase", phase)
			case <-time.After(400 * time.Millisecond):
			}
		}
	}
	wait("both inactive with full long-window coverage on healthy traffic", func(r map[string]liveRule) bool {
		if len(r) != 2 || r["critical"].State != "inactive" || r["warning"].State != "inactive" {
			return false
		}
		expression := fmt.Sprintf(`nines:slo_coverage:ratio{nines_tenant="acceptance",nines_source="production",nines_slo=%q,window="40s"} == 1`, slo.ID)
		code, body, err := rulerRequest(ctx, target, http.MethodGet, "/api/v1/query?query="+url.QueryEscape(expression), nil)
		var response struct {
			Status string `json:"status"`
			Data   struct {
				Result []json.RawMessage `json:"result"`
			} `json:"data"`
		}
		return err == nil && code == http.StatusOK && json.Unmarshal(body, &response) == nil && response.Status == "success" && len(response.Data.Result) == 1
	})
	setGood(90)
	independentPending := false
	wait("different pending durations then both firing", func(r map[string]liveRule) bool {
		if r["critical"].State == "firing" && r["warning"].State == "pending" {
			independentPending = true
		}
		return r["critical"].State == "firing" && r["warning"].State == "firing"
	})
	if !independentPending || !fired["critical"] || !fired["warning"] {
		t.Fatal("did not prove independent pending durations")
	}
	setGood(99.5) // 5x burn: below critical 8x, above warning 2x.
	var recovered time.Time
	wait("critical recovers while warning stays firing across multiple evaluations", func(r map[string]liveRule) bool {
		if r["critical"].State != "inactive" || r["warning"].State != "firing" {
			recovered = time.Time{}
			return false
		}
		if recovered.IsZero() {
			recovered = r["critical"].LastEvaluation
		}
		return r["critical"].LastEvaluation.Sub(recovered) >= 2*time.Duration(slo.StepSeconds)*time.Second
	})
	setGood(100)
	wait("warning recovers independently after healthy input", func(r map[string]liveRule) bool {
		return len(r) == 2 && r["critical"].State == "inactive" && r["warning"].State == "inactive"
	})
	deployment, evaluation := manager.Inspect(ctx, slo, time.Now().UTC())
	if deployment.State != "stored" || evaluation.State != "evaluating" {
		t.Fatal("definition drifted during lifecycle", deployment, evaluation)
	}
	t.Log("Both policies retained the exact reviewed rule group and revision 3 through pending, firing and independent recovery")
}
