package operator_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/operator"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

type clusterFixture struct {
	t         *testing.T
	ctx       context.Context
	container string
	api       *operator.API
	mimir     string
}

func realCluster(t *testing.T) *clusterFixture {
	t.Helper()
	name := os.Getenv("NINES_OPERATOR_TEST_CONTAINER")
	if name == "" {
		t.Skip("run make test-operator for the disposable real API/controller fixture")
	}
	if !strings.HasPrefix(name, "nines-operator-acceptance-") {
		t.Fatal("refusing non-fixture container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	owned, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "nines.acceptance"}}`, name).Output()
	if err != nil || strings.TrimSpace(string(owned)) != "operator" {
		t.Fatal("not our disposable API container", err)
	}
	base := os.Getenv("NINES_OPERATOR_TEST_API")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" || u.Path != "" || u.User != nil || u.RawQuery != "" {
		t.Fatal("use the private fixture's loopback TLS origin")
	}
	published, err := exec.CommandContext(ctx, "docker", "port", name, "6443/tcp").Output()
	if err != nil || strings.TrimSpace(string(published)) != u.Host {
		t.Fatal("API origin is not this fixture's published port")
	}
	ca, err := os.ReadFile(os.Getenv("NINES_OPERATOR_TEST_CA"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("invalid fixture CA")
	}
	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}}
	t.Cleanup(func() { httpClient.CloseIdleConnections() })
	return &clusterFixture{t: t, ctx: ctx, container: name, api: &operator.API{Base: base, Namespace: "team-commerce", TokenFile: os.Getenv("NINES_OPERATOR_TEST_TOKEN"), HTTP: httpClient}, mimir: os.Getenv("NINES_OPERATOR_TEST_MIMIR")}
}
func (f *clusterFixture) command(input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(f.ctx, "docker", append([]string{"exec", "-i", f.container, "kubectl"}, args...)...)
	cmd.Stdin = bytes.NewReader(input)
	return cmd.CombinedOutput()
}
func (f *clusterFixture) kubectl(input []byte, args ...string) []byte {
	f.t.Helper()
	out, err := f.command(input, args...)
	if err != nil {
		f.t.Fatalf("fixture kubectl %v: %v: %s", args, err, out)
	}
	return out
}
func (f *clusterFixture) apply(value any) {
	f.t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		f.t.Fatal(err)
	}
	f.kubectl(data, "apply", "-f", "-")
}
func (f *clusterFixture) get(kind, name string) k.Resource {
	f.t.Helper()
	var r k.Resource
	if err := json.Unmarshal(f.kubectl(nil, "get", kind, name, "-n", "team-commerce", "-o", "json"), &r); err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *clusterFixture) wait(label string, condition func() bool) {
	f.t.Helper()
	for {
		if condition() {
			f.t.Log("Verified:", label)
			return
		}
		select {
		case <-f.ctx.Done():
			f.t.Fatal("fixture timeout:", label)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
func (f *clusterFixture) desired(name, source string) k.Resource {
	slo := model.SLO{ID: name, Source: source, Project: "commerce", Service: "checkout", Name: name, Target: .999, WindowSeconds: 60, StepSeconds: 1, GoodQuery: "vector(99)", TotalQuery: "vector(100)", Policy: &model.Policy{ShortWindowSeconds: 2, LongWindowSeconds: 4, BurnRate: 2, ForSeconds: 2, MinCoverage: 1}}
	d := model.DefinitionFromSLO(slo)
	r, err := operator.FromDefinition(model.Definition{SLO: &d}, "team-commerce")
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}
func isReady(r k.Resource, reason string) bool {
	return r.Status != nil && r.Status.ObservedGeneration == r.Metadata.Generation && len(r.Status.Conditions) > 0 && r.Status.Conditions[0].Status == "True" && r.Status.Conditions[0].Reason == reason
}

func TestRealOperatorControllerConformance(t *testing.T) {
	f := realCluster(t)
	// Structural admission and the exact shipped namespaced Role are exercised by
	// the real API, not a fake API handler or an admin-authenticated controller.
	invalid := f.desired("invalid", "primary")
	var invalidSpec map[string]any
	_ = json.Unmarshal(invalid.Spec, &invalidSpec)
	invalidSpec["target"] = "not-a-number"
	invalid.Spec, _ = json.Marshal(invalidSpec)
	body, _ := json.Marshal(invalid)
	if out, err := f.command(body, "create", "--validate=false", "-f", "-"); err == nil || !strings.Contains(string(out), "spec.target") {
		t.Fatal("structurally invalid CRD admitted")
	}
	if err := f.api.Probe(f.ctx); err != nil {
		t.Fatal("shipped Role cannot read SLOs", err)
	}
	cross := *f.api
	cross.Namespace = "nines-rules"
	if _, err := cross.List(f.ctx, v1.SLOKind); err == nil {
		t.Fatal("service-account Role leaked across namespaces")
	}
	for _, verb := range []string{"get secrets", "create clusterroles", "delete prometheusrules"} {
		fields := strings.Fields(verb)
		out, _ := f.command(nil, "auth", "can-i", fields[0], fields[1], "--as=system:serviceaccount:team-commerce:nines-operator", "-n", "team-commerce")
		if text := strings.TrimSpace(string(out)); text != "no" && !strings.HasSuffix(text, "\nno") {
			t.Fatal("operator has unneeded permission", verb, string(out))
		}
	}
	backend := httpTestUnavailable(t)
	c := &operator.Controller{API: f.api, Config: operator.Config{Namespace: "team-commerce", Tenant: "commerce", Projects: []string{"commerce"}, Sources: map[string]operator.Source{"primary": {Kind: "prometheus-operator", QueryURL: backend, PrometheusLabels: map[string]string{"nines-test-rule": "selected"}}}}}
	// Reconcile uses the production controller and actual service-account token.
	reconcile := func(name string) k.Resource {
		t.Helper()
		r := f.get("slo", name)
		if err := c.Reconcile(f.ctx, r); err != nil {
			t.Fatal("reconcile", name, err)
		}
		return f.get("slo", name)
	}
	// Admission must retain the shared fields, and reconciliation must replace
	// the complete rule group when paging is enabled or disabled.
	lifecycleResource := f.desired("lifecycle-guard", "primary")
	var lifecycleSpec v1.SLOSpec
	if err := json.Unmarshal(lifecycleResource.Spec, &lifecycleSpec); err != nil {
		t.Fatal(err)
	}
	lifecycleSpec.Policy.MinEvents = 150
	lifecycleSpec.Quality = &v1.MeasurementQualityPolicy{MinEvents: 100, CohortLabels: []string{"region"}}
	lifecycleSpec.ErrorBudgetPolicy = &v1.ErrorBudgetPolicy{Owner: "oncall", RunbookURL: "https://example.test/runbook", ReviewBelow: .25}
	for _, lifecycle := range []v1.SLOLifecycle{v1.LifecycleAspirational, v1.LifecycleCommitted} {
		lifecycleSpec.Lifecycle = lifecycle
		lifecycleResource.Spec, _ = json.Marshal(lifecycleSpec)
		f.apply(lifecycleResource)
		saved := reconcile("lifecycle-guard")
		native, err := operator.Native(saved)
		if err != nil || native.SLO.Spec.Lifecycle != lifecycle || native.SLO.Spec.Policy.MinEvents != 150 || native.SLO.Spec.Quality.MinEvents != 100 || native.SLO.Spec.ErrorBudgetPolicy.Owner != "oncall" {
			t.Fatal("admission pruned lifecycle or count", native, err)
		}
		child := f.get("prometheusrule", operator.RuleName("lifecycle-guard"))
		var groups rules.File
		if err = json.Unmarshal(child.Spec, &groups); err != nil {
			t.Fatal(err)
		}
		alertCount := 0
		for _, group := range groups.Groups {
			for _, rule := range group.Rules {
				if rule.Alert != "" {
					alertCount++
				}
			}
		}
		if (alertCount == 1) != (lifecycle == v1.LifecycleCommitted) {
			t.Fatal("operator paging lifecycle mismatch", lifecycle, alertCount)
		}
	}
	f.apply(f.desired("live-checkout", "primary"))
	current := reconcile("live-checkout")
	if !isReady(current, "RulesConfigured") || current.Status.Evaluation == nil || current.Status.Evaluation.State != "unknown" {
		t.Fatal("configuration/evaluation evidence conflated", current.Status)
	}
	child := f.get("prometheusrule", operator.RuleName("live-checkout"))
	if len(child.Metadata.OwnerReferences) != 1 || child.Metadata.OwnerReferences[0].UID != current.Metadata.UID {
		t.Fatal("child lost actual owner UID")
	}
	// Real API resource versions, status subresource and generation updates.
	beforeVersion := child.Metadata.ResourceVersion
	reconcile("live-checkout")
	if f.get("prometheusrule", child.Metadata.Name).Metadata.ResourceVersion != beforeVersion {
		t.Fatal("idempotent reconciliation rewrote child")
	}
	next := f.desired("live-checkout", "primary")
	var spec map[string]any
	_ = json.Unmarshal(next.Spec, &spec)
	spec["target"] = .99
	next.Spec, _ = json.Marshal(spec)
	stale := current
	f.apply(next)
	current = reconcile("live-checkout")
	if current.Metadata.Generation != 2 || !isReady(current, "RulesConfigured") {
		t.Fatal("new generation status not current", current)
	}
	child = f.get("prometheusrule", child.Metadata.Name)
	beforeStale := operator.CanonicalDigest(child.Spec)
	if err := c.Reconcile(f.ctx, stale); err == nil {
		t.Fatal("stale parent generation reconciled")
	}
	if operator.CanonicalDigest(f.get("prometheusrule", child.Metadata.Name).Spec) != beforeStale {
		t.Fatal("stale reconcile overwrote newer generation")
	}
	var file rules.File
	if err := json.Unmarshal(child.Spec, &file); err != nil {
		t.Fatal(err)
	}
	for _, rule := range file.Groups[0].Rules {
		if (rule.Alert != "" || rule.Record == model.RuleOutputMetric) && rule.Labels["nines_revision"] != "2" {
			t.Fatal("stale generation in compiled rules")
		}
	}
	// Edited/deleted owned output is reconciled, while foreign ownership is blocked.
	child.Spec = json.RawMessage(`{"groups":[{"name":"edited","rules":[{"record":"edited","expr":"vector(1)"}]}]}`)
	f.apply(child)
	reconcile("live-checkout")
	if string(f.get("prometheusrule", child.Metadata.Name).Spec) == string(child.Spec) {
		t.Fatal("owned drift was not repaired")
	}
	f.kubectl(nil, "delete", "prometheusrule", child.Metadata.Name, "-n", "team-commerce", "--wait=false")
	reconcile("live-checkout")
	foreign := f.desired("foreign", "primary")
	f.apply(foreign)
	f.apply(map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "PrometheusRule", "metadata": map[string]string{"name": operator.RuleName("foreign"), "namespace": "team-commerce"}, "spec": map[string]any{"groups": []any{}}})
	if err := c.Reconcile(f.ctx, f.get("slo", "foreign")); err == nil {
		t.Fatal("foreign resource was adopted")
	}
	if len(f.get("prometheusrule", operator.RuleName("foreign")).Metadata.OwnerReferences) != 0 {
		t.Fatal("foreign owner was changed")
	}
	// Same-namespace project denial is distinct from namespace RBAC.
	denied := f.desired("scope-denied", "primary")
	denied.Metadata.Annotations[k.ProjectAnnotation] = "other"
	f.apply(denied)
	if err := c.Reconcile(f.ctx, f.get("slo", "scope-denied")); err != nil {
		t.Fatal(err)
	}
	if r := f.get("slo", "scope-denied"); r.Status == nil || r.Status.Conditions[0].Reason != "ScopeDenied" {
		t.Fatal("project denial not visible")
	}
	// Kube-controller-manager performs actual owner-reference garbage collection.
	f.kubectl(nil, "delete", "slo", "live-checkout", "-n", "team-commerce", "--wait=false")
	f.wait("owner-reference garbage collection", func() bool {
		out, err := f.command(nil, "get", "prometheusrule", child.Metadata.Name, "-n", "team-commerce", "--ignore-not-found", "-o", "json")
		return err == nil && len(bytes.TrimSpace(out)) == 0
	})
	// All domain CRDs use the real API. Telemetry here is deliberately a local
	// metadata fixture; this tests deterministic compilation and generation pins.
	telemetry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/api/v1/metadata":
			data = map[string]any{"http_requests_total": []any{map[string]string{"type": "counter"}}}
		case "/api/v1/series":
			data = []map[string]string{{"__name__": "http_requests_total", "service": "api", "status": "200"}, {"__name__": "http_requests_total", "service": "api", "status": "500"}}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": data})
	}))
	defer telemetry.Close()
	originalPrimary := c.Config.Sources["primary"]
	c.Config.Sources["primary"] = operator.Source{Kind: "prometheus-operator", QueryURL: telemetry.URL}
	binding := testtelemetry.Binding()
	binding.Metadata.Project = "commerce"
	binding.Metadata.Name = "real-binding"
	br, err := operator.FromDefinition(model.Definition{Binding: &binding}, "team-commerce")
	if err != nil {
		t.Fatal(err)
	}
	f.apply(br)
	if err := c.Reconcile(f.ctx, f.get("measurementbinding", "real-binding")); err != nil {
		t.Fatal(err)
	}
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "real-intent", Project: "commerce"}, Spec: v1.IntentSpec{Description: "99.9% of API requests succeed", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}
	ir, err := operator.FromIntent(goal, "team-commerce", k.MeasurementSelection{Name: "real-binding", ExpectedGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.apply(ir)
	if err := c.Reconcile(f.ctx, f.get("servicelevelobjectiveintent", "real-intent")); err != nil {
		t.Fatal(err)
	}
	if !isReady(f.get("servicelevelobjectiveintent", "real-intent"), "IntentCompiled") {
		t.Fatal("intent status not persisted")
	}
	compiled := f.get("slo", "real-intent")
	if len(compiled.Metadata.OwnerReferences) != 1 || compiled.Metadata.OwnerReferences[0].UID != f.get("servicelevelobjectiveintent", "real-intent").Metadata.UID {
		t.Fatal("intent child has incorrect owner")
	}
	if err := c.Reconcile(f.ctx, compiled); err != nil {
		t.Fatal(err)
	}
	f.kubectl(nil, "patch", "measurementbinding", "real-binding", "-n", "team-commerce", "--type=merge", "-p", `{"spec":{"observationPoint":"Revised observation contract"}}`)
	if err := c.Reconcile(f.ctx, f.get("servicelevelobjectiveintent", "real-intent")); err == nil {
		t.Fatal("binding generation changed without explicit selection")
	}
	if operator.CanonicalDigest(f.get("slo", "real-intent").Spec) != operator.CanonicalDigest(compiled.Spec) {
		t.Fatal("stale binding approval changed compiled SLO")
	}
	f.kubectl(nil, "delete", "servicelevelobjectiveintent", "real-intent", "-n", "team-commerce", "--wait=false")
	f.wait("intent-to-SLO-to-rule garbage collection", func() bool {
		out, err := f.command(nil, "get", "slo/real-intent", "prometheusrule/"+operator.RuleName("real-intent"), "-n", "team-commerce", "--ignore-not-found", "-o", "json")
		if err != nil {
			return false
		}
		if len(bytes.TrimSpace(out)) == 0 {
			return true
		}
		var list struct {
			Items []json.RawMessage `json:"items"`
		}
		return json.Unmarshal(out, &list) == nil && len(list.Items) == 0
	})
	c.Config.Sources["primary"] = originalPrimary

	// Mimir lifecycle uses the same CRD/status/finalizer API with real ruler storage.
	f.wait("disposable Mimir ready", func() bool {
		req, _ := http.NewRequestWithContext(f.ctx, http.MethodGet, f.mimir+"/ready", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == 200
	})
	c.Config.Sources["mimir"] = operator.Source{Kind: "mimir", QueryURL: f.mimir + "/prometheus", RulerURL: f.mimir + "/prometheus", MimirOrgID: "operator-conformance", RulerNamespace: "owned-rules"}
	f.apply(f.desired("mimir-checkout", "mimir"))
	f.wait("Mimir finalizer, recorded destination and accepted owned rules", func() bool { r := reconcile("mimir-checkout"); return isReady(r, "RulesAccepted") })
	f.wait("current-generation Mimir runtime and query output", func() bool {
		r := reconcile("mimir-checkout")
		return r.Status.Evaluation != nil && r.Status.Evaluation.State == "evaluating" && r.Status.RuleOutput != nil && r.Status.RuleOutput.State == "verified"
	})
	mr := f.get("slo", "mimir-checkout")
	if len(mr.Metadata.Finalizers) != 1 || mr.Status.ManagedGroup == "" {
		t.Fatal("cleanup destination not persisted before write")
	}
	originalRoute := c.Config.Sources["mimir"]
	wrong := originalRoute
	wrong.RulerNamespace = "different"
	c.Config.Sources["mimir"] = wrong
	f.kubectl(nil, "delete", "slo", "mimir-checkout", "-n", "team-commerce", "--wait=false")
	if err := c.Reconcile(f.ctx, f.get("slo", "mimir-checkout")); err == nil {
		t.Fatal("cleanup followed changed route")
	}
	if blocked := f.get("slo", "mimir-checkout"); len(blocked.Metadata.Finalizers) != 1 || blocked.Status.Conditions[0].Reason != "CleanupBlocked" {
		t.Fatal("blocked finalizer not visible")
	}
	c.Config.Sources["mimir"] = originalRoute
	if err := c.Reconcile(f.ctx, f.get("slo", "mimir-checkout")); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(f.ctx, http.MethodGet, f.mimir+"/prometheus/config/v1/rules/owned-rules/"+mr.Status.ManagedGroup, nil)
	req.Header.Set("X-Scope-OrgID", "operator-conformance")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("owned Mimir group was not removed")
	}
	t.Log("Real admission, shipped RBAC, revision/status, idempotency, drift repair, foreign ownership, garbage collection, Mimir acceptance and guarded finalizer cleanup passed")
}

func httpTestUnavailable(t *testing.T) string {
	t.Helper()
	// No production endpoint is queried in this conformance test. The healthy
	// provider/runtime contract has separate real Mimir acceptance coverage.
	return "http://127.0.0.1:1"
}

func TestRealOperatorLeaseFailover(t *testing.T) {
	f := realCluster(t)
	backend := httpTestUnavailable(t)
	config := operator.Config{Namespace: "team-commerce", Tenant: "commerce", Projects: []string{"commerce"}, Sources: map[string]operator.Source{"primary": {Kind: "prometheus-operator", QueryURL: backend}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	firstAPI := *f.api
	first := &operator.Controller{API: &firstAPI, Config: config}
	firstCtx, stop := context.WithCancel(f.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := first.Run(firstCtx, logger); err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	t.Cleanup(func() {
		stop()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("first controller did not stop")
		}
	})
	f.apply(f.desired("failover", "primary"))
	f.wait("first leader configures desired resource", func() bool { return isReady(f.get("slo", "failover"), "RulesConfigured") })
	var firstLease struct {
		Spec struct {
			Holder string `json:"holderIdentity"`
		} `json:"spec"`
	}
	_ = json.Unmarshal(f.kubectl(nil, "get", "lease", "nines-operator", "-n", "team-commerce", "-o", "json"), &firstLease)
	secondAPI := *f.api
	second := &operator.Controller{API: &secondAPI, Config: config}
	secondCtx, stopSecond := context.WithCancel(f.ctx)
	doneSecond := make(chan struct{})
	go func() {
		defer close(doneSecond)
		if err := second.Run(secondCtx, logger); err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	t.Cleanup(func() {
		stopSecond()
		select {
		case <-doneSecond:
		case <-time.After(10 * time.Second):
			t.Error("standby controller did not stop")
		}
	})
	// An external holder takes the lease. Renewal must cancel the old leader and
	// fence its write guard before any controller can continue reconciling.
	patch := `{"spec":{"holderIdentity":"external-holder","renewTime":"2099-01-01T00:00:00.000000Z"}}`
	f.kubectl(nil, "patch", "lease", "nines-operator", "-n", "team-commerce", "--type=merge", "-p", patch)
	next := f.desired("failover", "primary")
	var nextSpec map[string]any
	_ = json.Unmarshal(next.Spec, &nextSpec)
	nextSpec["target"] = .99
	next.Spec, _ = json.Marshal(nextSpec)
	f.apply(next)
	time.Sleep(12 * time.Second)
	blocked := f.get("slo", "failover")
	if blocked.Metadata.Generation != 2 || blocked.Status == nil || blocked.Status.ObservedGeneration != 1 {
		t.Fatal("a controller reconciled after external lease revocation", blocked.Status)
	}
	stop()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("leader did not cancel")
	}
	if firstAPI.BeforeWrite == nil || firstAPI.BeforeWrite() == nil {
		t.Fatal("revoked leader can still write")
	}
	// Expire the external lease and prove a different real controller takes over.
	f.kubectl(nil, "patch", "lease", "nines-operator", "-n", "team-commerce", "--type=merge", "-p", `{"spec":{"renewTime":"2000-01-01T00:00:00.000000Z","leaseDurationSeconds":1}}`)
	f.wait("standby acquires lease and reconciles generation 2", func() bool {
		r := f.get("slo", "failover")
		return r.Metadata.Generation == 2 && isReady(r, "RulesConfigured")
	})
	var after struct {
		Spec struct {
			Holder string `json:"holderIdentity"`
		} `json:"spec"`
	}
	_ = json.Unmarshal(f.kubectl(nil, "get", "lease", "nines-operator", "-n", "team-commerce", "-o", "json"), &after)
	if after.Spec.Holder == "external-holder" || after.Spec.Holder == firstLease.Spec.Holder || after.Spec.Holder == "" {
		t.Fatal("no real leader handover")
	}
	t.Log("Lease revocation fenced the former leader; a distinct standby reconciled the current generation")
}
