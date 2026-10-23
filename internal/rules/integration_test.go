package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishantanu/nines/internal/prometheus"
)

func TestRealMimirRuleDestinations(t *testing.T) {
	base := strings.TrimRight(os.Getenv("MIMIR_TEST_URL"), "/")
	if base == "" {
		t.Skip("set MIMIR_TEST_URL to the disposable loopback Mimir fixture")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("use a disposable loopback origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("refusing non-loopback rule writes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	namespace := fmt.Sprintf("nines-test-%d", time.Now().UnixNano())
	groups := []string{}
	inspections := []func() error{}
	sloID := "rules-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	for i, org := range []string{"nines-rules-a", "nines-rules-b"} {
		slo := fixture()
		slo.ID = sloID
		slo.GoodQuery = "vector(99)"
		slo.TotalQuery = "vector(100)"
		slo.StepSeconds = 1
		slo.WindowSeconds = 60
		slo.Policy.ShortWindowSeconds = 2
		slo.Policy.LongWindowSeconds = 4
		slo.Policy.ForSeconds = 2
		slo.Policy.BurnRate = 2
		if i == 0 {
			slo.Target = .99
		}
		manager := Manager{Tenant: []string{"commerce", "analytics"}[i], Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", QueryURL: base + "/prometheus", RulerURL: base + "/prometheus", MimirOrgID: org, Namespace: namespace}}}
		plan, err := manager.Plan(slo)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.Deploy(ctx, slo, plan.Digest)
		if err != nil || result.State != "accepted" || result.Activation == nil || result.Activation.State != "accepted" {
			t.Fatal(result, err)
		}
		groups = append(groups, plan.Group.Name)
		inspections = append(inspections, func() error {
			deployment, evaluation := manager.Inspect(ctx, slo, time.Now().UTC())
			if deployment.State != "stored" || evaluation.State != "evaluating" {
				return fmt.Errorf("real Mimir inspection not current: %s/%s (%s/%s)", deployment.State, evaluation.State, deployment.Code, evaluation.Code)
			}
			identity := OutputIdentity(slo, manager.Tenant, manager.OwnerUID)
			collector, err := prometheus.NewMimir(base+"/prometheus", org, "")
			if err != nil {
				return err
			}
			output := collector.RuleOutput(ctx, identity, time.Now().UTC())
			if output.State != "verified" {
				return fmt.Errorf("current output is %s/%s", output.State, output.Code)
			}
			// The ruler remains healthy when a query gateway selects another
			// dataset. Never infer query visibility from runtime evaluation.
			wrong, err := prometheus.NewMimir(base+"/prometheus", "nines-query-dataset-without-rules", "")
			if err != nil {
				return err
			}
			missing := wrong.RuleOutput(ctx, identity, time.Now().UTC())
			if missing.Code != "rule_output_missing" {
				return fmt.Errorf("wrong dataset output: %s", missing.Code)
			}
			identity.Revision++
			old := collector.RuleOutput(ctx, identity, time.Now().UTC())
			if old.Code != "rule_output_out_of_date" {
				return fmt.Errorf("other revision output: %s", old.Code)
			}
			return nil
		})
		code, _, err := rulerRequest(ctx, manager.Targets["production"], http.MethodGet, "/config/v1/rules/"+namespace+"/"+plan.Group.Name, nil)
		if err != nil || code != 200 {
			t.Fatal("rule group not stored in selected org", code, err)
		}
	}
	target := Target{RulerURL: base + "/prometheus", MimirOrgID: "nines-rules-b"}
	code, _, err := rulerRequest(ctx, target, http.MethodGet, "/config/v1/rules/"+namespace+"/"+groups[0], nil)
	if err != nil || code != 404 {
		t.Fatal("cross-dataset rule leakage", code, err)
	}
	query := func(org, expression string) ([]float64, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/prometheus/api/v1/query?query="+url.QueryEscape(expression), nil)
		req.Header.Set("X-Scope-OrgID", org)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		var envelope struct {
			Status string `json:"status"`
			Data   struct {
				Result []struct {
					Value []json.RawMessage `json:"value"`
				} `json:"result"`
			} `json:"data"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			return nil, err
		}
		if envelope.Status != "success" {
			return nil, fmt.Errorf("query failed HTTP %d", resp.StatusCode)
		}
		values := []float64{}
		for _, series := range envelope.Data.Result {
			if len(series.Value) != 2 {
				continue
			}
			var text string
			if err := json.Unmarshal(series.Value[1], &text); err != nil {
				t.Fatal(err)
			}
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		return values, nil
	}
	scope := `nines_source="production",nines_slo="` + sloID + `"`
	for {
		a, ea := query("nines-rules-a", `nines:slo_target:ratio{`+scope+`}`)
		b, eb := query("nines-rules-b", `nines:slo_target:ratio{`+scope+`}`)
		burnA, ba := query("nines-rules-a", `nines:slo_burn_rate:ratio{`+scope+`,window="2s"}`)
		burnB, bb := query("nines-rules-b", `nines:slo_burn_rate:ratio{`+scope+`,window="2s"}`)
		alertA, aa := query("nines-rules-a", `ALERTS{`+scope+`,alertname="NinesSLOBurnRate",alertstate="firing"}`)
		alertB, ab := query("nines-rules-b", `ALERTS{`+scope+`,alertname="NinesSLOBurnRate",alertstate="firing"}`)
		var inspectionErr error
		for _, inspect := range inspections {
			if err := inspect(); err != nil {
				inspectionErr = err
				break
			}
		}
		if inspectionErr == nil && ea == nil && eb == nil && ba == nil && bb == nil && aa == nil && ab == nil && len(a) == 1 && len(b) == 1 && a[0] == .99 && b[0] == .999 && len(burnA) == 1 && len(burnB) == 1 && math.Abs(burnA[0]-1) < 1e-8 && math.Abs(burnB[0]-10) < 1e-8 && len(alertA) == 0 && len(alertB) == 1 && alertB[0] == 1 {
			t.Log("Separate Mimir datasets: targets .99/.999, burn 1x/10x, only the second tenant firing after both windows and pending duration")
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("ruler did not evaluate isolated target series", a, b, ea, eb, inspectionErr)
		case <-time.After(time.Second):
		}
	}
}
