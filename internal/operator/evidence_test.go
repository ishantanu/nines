package operator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

func TestOperatorQueryAndRulerCredentialsStaySeparate(t *testing.T) {
	dir := t.TempDir()
	queryFile, rulerFile := filepath.Join(dir, "query"), filepath.Join(dir, "ruler")
	if err := os.WriteFile(queryFile, []byte("query-only"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulerFile, []byte("ruler-only"), 0600); err != nil {
		t.Fatal(err)
	}
	var seen atomic.Bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(true)
		if r.Header.Get("Authorization") != "Bearer query-only" || r.Header.Get("X-Scope-OrgID") != "commerce" {
			t.Error("wrong query credential or org")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer backend.Close()
	source := Source{Kind: "mimir", QueryURL: backend.URL, RulerURL: backend.URL, MimirOrgID: "commerce", RulerNamespace: "nines", QueryBearerTokenFile: queryFile, RulerBearerTokenFile: rulerFile}
	collector, err := source.Collector()
	if err != nil {
		t.Fatal(err)
	}
	stage := collector.RuleOutput(context.Background(), rules.OutputIdentity(definition().SLO(), "commerce", "owner"), time.Now().UTC())
	if !seen.Load() || stage.Code != "rule_output_missing" {
		t.Fatal(stage)
	}
	target, err := source.Target("primary")
	if err != nil || target.BearerToken != "ruler-only" {
		t.Fatal("write credential mixed with query", err)
	}
	if err := os.WriteFile(queryFile, []byte("rotated-query"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := source.Collector()
	if err != nil || next.BearerToken != "rotated-query" {
		t.Fatal("mounted token did not rotate", err)
	}
}

func TestOperatorScopeFailureClearsEarlierProviderEvidence(t *testing.T) {
	c, f := controller(t)
	r := sloResource(t)
	r.Metadata.Annotations["nines.dev/project"] = "denied"
	r.Status = &k.Status{Deployment: &v1.IntegrationStage{State: "deployed"}, Evaluation: &v1.IntegrationStage{State: "evaluating"}, RuleOutput: &v1.IntegrationStage{State: "verified"}}
	f.put(r)
	if err := c.Reconcile(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	status := f.get(r.Kind, r.Metadata.Name).Status
	if status.Conditions[0].Reason != "ScopeDenied" || status.Evaluation != nil || status.RuleOutput != nil || status.Deployment != nil {
		t.Fatal("old positive evidence survived denial", status)
	}
}

func TestStaleParentCannotOverwriteCurrentRuleGeneration(t *testing.T) {
	c, f := controller(t)
	stale := sloResource(t)
	f.put(stale)
	if err := c.Reconcile(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	current := f.get(stale.Kind, stale.Metadata.Name)
	var native model.Definition
	var err error
	native, err = Native(current)
	if err != nil {
		t.Fatal(err)
	}
	native.SLO.Spec.Target = .99
	desired, err := FromDefinition(native, current.Metadata.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	current.Spec = desired.Spec
	current.Metadata.Generation++
	current.Metadata.ResourceVersion = "new-desired"
	f.put(current)
	if err := c.Reconcile(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	before := CanonicalDigest(f.get("PrometheusRule", RuleName(current.Metadata.Name)).Spec)
	if err := c.Reconcile(context.Background(), stale); !errors.Is(err, model.ErrRevisionConflict) {
		t.Fatal("stale resource was not fenced", err)
	}
	if CanonicalDigest(f.get("PrometheusRule", RuleName(current.Metadata.Name)).Spec) != before {
		t.Fatal("stale reconcile overwrote current rules")
	}
}

func TestLeaseTimeUsesKubernetesMicroTimeWireFormat(t *testing.T) {
	for _, test := range []struct {
		nanos int
		want  string
	}{
		{123456789, `"2026-10-22T00:00:00.123456Z"`},
		{123450000, `"2026-10-22T00:00:00.123450Z"`},
		{0, `"2026-10-22T00:00:00.000000Z"`},
	} {
		instant := time.Date(2026, 10, 22, 0, 0, 0, test.nanos, time.UTC)
		body, err := json.Marshal(leaseTime{Time: instant})
		if err != nil || string(body) != test.want {
			t.Fatal("invalid Kubernetes MicroTime encoding", string(body), err)
		}
		var parsed leaseTime
		if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Equal(instant.Truncate(time.Microsecond)) {
			t.Fatal("lease timestamp did not round trip", err)
		}
	}
}
