package intentflow

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/prometheus"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

func TestRealPrometheusIntentActivation(t *testing.T) {
	base, dir := os.Getenv("PROMETHEUS_TEST_URL"), os.Getenv("PROMETHEUS_TEST_RULE_DIRECTORY")
	if base == "" || dir == "" {
		t.Skip("use make test-prometheus for the disposable fixture")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("use a private absolute rules directory")
	}
	realIntentActivation(t, "prometheus", base, dir)
}
func TestRealMimirIntentActivation(t *testing.T) {
	base := os.Getenv("MIMIR_TEST_URL")
	if base == "" {
		t.Skip("use make test-mimir for the disposable fixture")
	}
	realIntentActivation(t, "mimir", base, "")
}

// Seed the exact applied-operation references: approval/application are exercised
// independently by the headless transport tests. Publication, evaluation, output
// queries and collection below use the real disposable backend, never a mock.
func realIntentActivation(t *testing.T, kind, base, dir string) {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("use an isolated loopback origin")
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
		t.Fatal("refusing non-loopback rule publication")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	ctx = tenancy.WithPrincipal(ctx, tenancy.Principal{TenantID: "activation", Role: "owner", Subject: "synthetic-acceptance", Kind: "user"})
	ready, queryURL, org := "/-/ready", base, ""
	if kind == "mimir" {
		ready, queryURL, org = "/ready", base+"/prometheus", "nines-intent-activation-test"
	}
	client := http.Client{Timeout: time.Second}
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+ready, nil)
		res, e := client.Do(req)
		ok := e == nil && res.StatusCode == http.StatusOK
		if res != nil {
			_ = res.Body.Close()
		}
		if ok {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("backend not ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	var collector *prometheus.Client
	if kind == "mimir" {
		collector, err = prometheus.NewMimir(queryURL, org, "")
	} else {
		collector, err = prometheus.New(queryURL, "")
	}
	if err != nil {
		t.Fatal(err)
	}
	platform := service.New(store.NewMemory(), collector)
	platform.TenantID = "activation"
	slo := model.SLO{ID: fmt.Sprintf("intent-activation-%d", time.Now().UnixNano()), Project: "test", Service: "api", Name: "Synthetic request success", Source: "primary", Target: .999, StepSeconds: 1, WindowSeconds: 8, GoodQuery: "vector(99)", TotalQuery: "vector(100)"}
	if err = platform.Create(ctx, slo); err != nil {
		t.Fatal(err)
	}
	saved, err := platform.Store.Read(ctx, slo.ID)
	if err != nil {
		t.Fatal(err)
	}
	slo = saved.SLO
	target := rules.Target{Kind: kind, Connection: "primary", QueryURL: queryURL, RulerURL: queryURL, MimirOrgID: org, Namespace: "intent-acceptance", Directory: dir}
	manager := &rules.Manager{Tenant: platform.TenantID, Targets: map[string]rules.Target{"primary": target}}
	platform.RuleManager = manager
	flow := New(platform, NewMemory())
	in := input()
	in.Scope.Journey = nil
	in.Goal.Spec.Journey = nil
	in.ID = slo.ID
	in.Scope.Metadata.Name = slo.ID
	in.Goal.Metadata = in.Scope.Metadata
	in.Goal.Spec.WindowSeconds = slo.WindowSeconds
	in.Goal.Spec.StepSeconds = slo.StepSeconds
	conversation, err := flow.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	digest := hash(model.DefinitionFromSLO(slo))
	operationDigest := hash("synthetic-applied-operation-" + slo.ID)
	_, err = flow.Repository.Update(ctx, conversation.ID, func(r *record, now time.Time) error {
		r.Session.ChangeSet = &v1.IntentChangeSet{State: "applied", Digest: operationDigest, SourceDigest: platform.SourceFingerprint("primary"), Applied: []v1.IntentAppliedResource{{Kind: v1.SLOKind, Name: slo.ID, Revision: slo.Revision, Digest: digest}}}
		touch(r, now)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	inspect := func() v1.IntentActivation {
		t.Helper()
		out, e := flow.InspectActivation(ctx, conversation.ID)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	if out := inspect(); out.State == "verified" || out.Code != "publication_unverified" {
		t.Fatal("saved definition counted as activation", out)
	}
	plan, err := manager.Plan(slo)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := manager.Deploy(ctx, slo, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if kind == "prometheus" {
		if deployment.Activation == nil || deployment.Activation.State != "manual" {
			t.Fatal(deployment)
		}
		if out := inspect(); out.State == "verified" || out.Code != "evaluation_unverified" {
			t.Fatal("file write counted as activation", out)
		}
		target.Reload = true
		manager.Targets["primary"] = target
		plan, err = manager.Plan(slo)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = manager.Deploy(ctx, slo, plan.Digest); err != nil {
			t.Fatal(err)
		}
	}
	var last v1.IntentActivation
	for {
		last = inspect()
		if last.Code == "collection_unverified" {
			break
		}
		if last.State == "verified" {
			t.Fatal("rules alone counted as collected SLI", last)
		}
		if ctx.Err() != nil {
			t.Fatal("runtime/output did not become active", last)
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Interval-count queries return explicit synthetic counts. They require no
	// paid ingestion and do not pretend to measure real user traffic.
	end := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	if points, e := platform.Collect(ctx, slo.ID, end.Add(-8*time.Second), end); e != nil || points == 0 {
		t.Fatal("real query collection failed", points, e)
	}
	before, err := flow.Repository.Read(ctx, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	last = inspect()
	if last.State != "verified" || last.OperationDigest != operationDigest || last.Measurement == nil || last.Measurement.Good/last.Measurement.Total >= slo.Target {
		t.Fatal("activation must be independent of target attainment", last)
	}
	after, err := flow.Repository.Read(ctx, conversation.ID)
	if err != nil || hash(before) != hash(after) {
		t.Fatal("inspection mutated workflow", err)
	}
	if kind == "mimir" {
		target.MimirOrgID = "nines-intent-wrong-tenant"
		manager.Targets["primary"] = target
		if out := inspect(); out.State == "verified" || out.Code != "publication_unverified" {
			t.Fatal("foreign Mimir tenant counted as active", out)
		}
	}
	t.Log("Exact applied revision verified against real publication, evaluation, query output and collected SLI; definition-only/file-only states remained pending; below-target SLI did not alter activation semantics.")
}
