package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func TestPrometheusRetirementRequiresOwnedFileAndRuntimeAbsence(t *testing.T) {
	mode := "outage"
	reloads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/reload" {
			reloads++
			w.WriteHeader(200)
			return
		}
		if mode == "outage" {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	target := Target{Kind: "prometheus", Connection: "primary", Namespace: "tenant", Directory: directory, QueryURL: server.URL, Reload: true}
	manager := Manager{Tenant: "test", Targets: map[string]Target{"primary": target}}
	slo := model.SLO{ID: "retire", Source: "primary", Project: "p", Service: "api", Name: "API", Target: .99, StepSeconds: 10, WindowSeconds: 60, Revision: 1, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	plan, e := manager.Plan(slo)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(directory, target.Namespace+"-"+plan.Group.Name+".yaml")
	if e = os.WriteFile(path, []byte(plan.YAML), 0644); e != nil {
		t.Fatal(e)
	}
	slo.Retirement = &v1.SLORetirement{OriginalRevision: 1, RouteDigest: RouteDigest(target), Group: plan.Group.Name, Cleanup: "pending"}
	slo.Revision = 2
	done, e := manager.Retire(context.Background(), slo)
	if done || e != nil {
		t.Fatal("outage claimed absence", done, e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("owned file not removed", e)
	}
	mode = "absent"
	done, e = manager.Retire(context.Background(), slo)
	if !done || e != nil || reloads < 2 {
		t.Fatal("retry did not verify absence", done, e, reloads)
	}
	if e = os.WriteFile(path, []byte(strings.ReplaceAll(plan.YAML, "nines_tenant: test", "nines_tenant: foreign")), 0644); e != nil {
		t.Fatal(e)
	}
	done, e = manager.Retire(context.Background(), slo)
	if done || e == nil {
		t.Fatal("foreign group removed", done, e)
	}
	target.QueryURL = server.URL + "/changed"
	manager.Targets["primary"] = target
	if _, e = manager.Retire(context.Background(), slo); e == nil {
		t.Fatal("changed destination followed")
	}
}

type retirementPublication struct {
	uid    string
	writes int
}

func (p *retirementPublication) Plan(_ context.Context, _ model.SLO, t Target) (Plan, error) {
	return Plan{Target: t, Group: Group{Name: "owned"}, PriorUID: p.uid, Digest: "reviewed"}, nil
}
func (p *retirementPublication) Apply(context.Context, model.SLO, Plan) (Deployment, error) {
	p.writes++
	return Deployment{}, nil
}
func (p *retirementPublication) Inspect(context.Context, model.SLO, Target, time.Time) (v1.IntegrationStage, v1.IntegrationStage) {
	return v1.IntegrationStage{}, v1.IntegrationStage{}
}
func (p *retirementPublication) Identity(context.Context, model.SLO, Target) (model.RuleOutputIdentity, error) {
	return model.RuleOutputIdentity{}, nil
}
func (p *retirementPublication) Withdraw(context.Context, model.SLO, Plan) (Deployment, error) {
	p.writes++
	return Deployment{State: "withdrawing"}, nil
}
func TestRetirementDoesNotWithdrawAReplacementOperatorUID(t *testing.T) {
	target := Target{Kind: "operator", Connection: "primary", Publisher: "cluster", ProviderKind: "mimir"}
	publisher := &retirementPublication{uid: "replacement"}
	manager := Manager{Tenant: "default", Targets: map[string]Target{"primary": target}, External: publisher}
	archived := model.SLO{ID: "retire", Retirement: &v1.SLORetirement{OriginalRevision: 1, ResourceUID: "original", RouteDigest: RouteDigest(target)}}
	if done, e := manager.Retire(context.Background(), archived); e == nil || done || publisher.writes != 0 {
		t.Fatal("replacement publication was withdrawn", done, e, publisher.writes)
	}
	archived.Retirement.ResourceUID = ""
	if _, e := manager.Retire(context.Background(), archived); e == nil || publisher.writes != 0 {
		t.Fatal("new publication appeared after a no-publication review", e)
	}
	publisher.uid = "original"
	archived.Retirement.ResourceUID = "original"
	if done, e := manager.Retire(context.Background(), archived); e != nil || done || publisher.writes != 1 {
		t.Fatal("original publication could not begin cleanup", done, e, publisher.writes)
	}
}
