package rules

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ishantanu/nines/internal/model"
)

func fixture() model.SLO {
	return model.SLO{ID: "availability", Source: "production", Project: "p", Service: "api", Name: "API", Target: .999, WindowSeconds: 86400, StepSeconds: 60, GoodQuery: "sum(increase(requests_good[1m]))", TotalQuery: "sum(increase(requests_total[1m]))", Policy: &model.Policy{ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, ForSeconds: 120, MinCoverage: 1}}
}
func TestRuleFileOwnershipIdempotencyAndDestinationDigest(t *testing.T) {
	s := fixture()
	m := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "prometheus", Namespace: "nines", Directory: t.TempDir(), QueryURL: "https://metrics.example"}}}
	p, err := m.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Apply(context.Background(), s, "wrong"); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal(err)
	}
	r, err := m.Apply(context.Background(), s, p.Digest)
	if err != nil || r.State != "written" {
		t.Fatal(r, err)
	}
	r, err = m.Apply(context.Background(), s, p.Digest)
	if err != nil || r.State != "unchanged" {
		t.Fatal(r, err)
	}
	target := m.Targets["production"]
	target.Namespace = "another"
	m.Targets["production"] = target
	if _, err = m.Apply(context.Background(), s, p.Digest); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("destination changed without invalidating plan", err)
	}
	target.Namespace = "nines"
	m.Targets["production"] = target
	file := filepath.Join(target.Directory, target.Namespace+"-"+p.Group.Name+".yaml")
	if err = os.WriteFile(file, []byte("groups:\n- name: foreign\n  rules:\n  - record: foreign\n    expr: vector(1)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Apply(context.Background(), s, p.Digest); err == nil {
		t.Fatal("foreign rules overwritten")
	}
}
func TestMimirRulesUsePinnedOrgAndNamespaceWithNoRedirects(t *testing.T) {
	s := fixture()
	var writes atomic.Int64
	var group []byte
	ruler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Scope-OrgID") != "production-org" || r.Header.Get("Authorization") != "Bearer rule-secret" {
			t.Error("wrong dataset/auth")
		}
		if !strings.HasPrefix(r.URL.Path, "/prometheus/config/v1/rules/nines") {
			t.Error("wrong namespace", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			if group == nil {
				w.WriteHeader(404)
			} else {
				_, _ = w.Write(group)
			}
			return
		}
		if r.Header.Get("Content-Type") != "application/yaml" {
			t.Error("wrong media")
		}
		group, _ = io.ReadAll(r.Body)
		writes.Add(1)
		w.WriteHeader(202)
	}))
	defer ruler.Close()
	m := Manager{Tenant: "commerce", Targets: map[string]Target{"production": {Connection: "production", Kind: "mimir", QueryURL: ruler.URL + "/prometheus", RulerURL: ruler.URL + "/prometheus", MimirOrgID: "production-org", Namespace: "nines", BearerToken: "rule-secret"}}}
	p, err := m.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := yaml.Marshal(p.Group)
	if !strings.Contains(string(b), "nines_source: production") || !strings.Contains(string(b), "nines_tenant: commerce") {
		t.Fatal(string(b))
	}
	r, err := m.Apply(context.Background(), s, p.Digest)
	if err != nil || r.State != "accepted" {
		t.Fatal(r, err)
	}
	r, err = m.Apply(context.Background(), s, p.Digest)
	if err != nil || r.State != "unchanged" || writes.Load() != 1 {
		t.Fatal(r, err)
	}
	var destinationCalls atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	target := m.Targets["production"]
	target.RulerURL = redirect.URL
	m.Targets["production"] = target
	p, _ = m.Plan(s)
	if _, err = m.Apply(context.Background(), s, p.Digest); err == nil || destinationCalls.Load() != 0 {
		t.Fatal("redirect followed", err)
	}
}
func TestGeneratedRulesDurationsAndNoTrafficGuards(t *testing.T) {
	s := fixture()
	g, err := Generate(s, "commerce")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rules) != 17 {
		t.Fatal("unexpected recording/alert count", len(g.Rules))
	}
	for _, r := range g.Rules {
		if r.Labels["nines_source"] != "production" || r.Labels["nines_slo"] != s.ID {
			t.Fatal(r)
		}
	}
	last := g.Rules[len(g.Rules)-1]
	if last.Alert == "" || last.For != "120s" || !strings.Contains(last.Expr, ">= 14.4") || !strings.Contains(last.Expr, "window=\"300s\"") {
		t.Fatal(last)
	}
	for _, text := range []string{"28d", "1h30m", "0s", "1w2d"} {
		if _, err := DurationSeconds(text); err != nil {
			t.Fatal(text, err)
		}
	}
	for _, text := range []string{"-1h", "1.5h", "9000000000000000w", "2", ""} {
		if _, err := DurationSeconds(text); err == nil {
			t.Fatal("invalid duration accepted", text)
		}
	}
	if !strings.Contains(g.Rules[0].Expr, "count((") || !strings.Contains(g.Rules[3].Expr, "> 0") {
		t.Fatal("cardinality or zero-traffic guard missing")
	}
}

func TestOperatorUIDPreventsPlatformOverwriteAndForeignCleanup(t *testing.T) {
	g, err := Generate(fixture(), "commerce")
	if err != nil {
		t.Fatal(err)
	}
	if !ownedUID(g, "") || ownedUID(g, "operator-one") {
		t.Fatal("legacy ownership fence")
	}
	for i := range g.Rules {
		g.Rules[i].Labels["nines_resource_uid"] = "operator-one"
	}
	if ownedUID(g, "") || !ownedUID(g, "operator-one") || ownedUID(g, "operator-two") {
		t.Fatal("operator UID fence")
	}
}

func TestOwnedIntervalRulesScopeIntermediateQueriesByUID(t *testing.T) {
	for _, uid := range []string{"resource-one", "resource-two"} {
		group, e := GenerateOwned(fixture(), "commerce", uid)
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range group.Rules {
			if r.Labels["nines_resource_uid"] != uid {
				t.Fatal("unfenced output", r)
			}
			if strings.Contains(r.Expr, "nines:sli_good:interval{") || strings.Contains(r.Expr, "nines:sli_total:interval{") || strings.Contains(r.Expr, "nines:slo_burn_rate:ratio{") {
				if !strings.Contains(r.Expr, `nines_resource_uid="`+uid+`"`) {
					t.Fatal("cross-resource recording input", r.Expr)
				}
			}
		}
	}
}
