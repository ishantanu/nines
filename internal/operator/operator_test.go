package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/testtelemetry"
)

func definition() model.SLODefinition {
	return model.SLODefinition{APIVersion: v1.APIVersion, Kind: v1.SLOKind, Metadata: v1.Metadata{Name: "checkout", Project: "commerce"}, Spec: v1.SLOSpec{DisplayName: "Checkout", Service: "api", Source: "primary", Target: .999, WindowSeconds: 86400, StepSeconds: 60, GoodQuery: "sum(increase(requests_total{status=\"200\"}[1m]))", TotalQuery: "sum(increase(requests_total[1m]))", Policy: &v1.BurnRatePolicy{ShortWindowSeconds: 300, LongWindowSeconds: 3600, BurnRate: 14.4, MinCoverage: 1}}}
}
func TestRenderSharesCompilerAndPreservesDomainVersion(t *testing.T) {
	d := definition()
	resource, e := FromDefinition(model.Definition{SLO: &d}, "team-commerce")
	if e != nil {
		t.Fatal(e)
	}
	native, e := Native(resource)
	if e != nil || model.DefinitionDigest(native.Manifest()) != model.DefinitionDigest(d) {
		t.Fatal(native, e)
	}
	rendered, e := PrometheusRule(d, "commerce", "team-commerce", "owner-uid", map[string]string{"prometheus": "primary"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	var output rules.File
	if e = json.Unmarshal(rendered.Spec, &output); e != nil {
		t.Fatal(e)
	}
	expected, e := rules.GenerateOwned(d.SLO(), "commerce", "owner-uid")
	if e != nil || model.DefinitionDigest(output.Groups[0]) != model.DefinitionDigest(expected) {
		t.Fatal("operator diverged from core rule math", e)
	}
	if !owned(rendered, "owner-uid") || rendered.Metadata.Namespace != "team-commerce" || rendered.Metadata.Labels["prometheus"] != "primary" {
		t.Fatal(rendered)
	}
	for _, crd := range CRDs() {
		b, e := YAML(crd)
		if e != nil {
			t.Fatal(e)
		}
		var roundtrip map[string]any
		if yaml.Unmarshal(b, &roundtrip) != nil || roundtrip["kind"] != "CustomResourceDefinition" {
			t.Fatal("invalid CRD")
		}
	}
}

type kubeFixture struct {
	mu      sync.Mutex
	objects map[string]k.Resource
	next    int
	writes  int
}

func newKube(t *testing.T) (*API, *kubeFixture) {
	t.Helper()
	f := &kubeFixture{objects: map[string]k.Resource{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return &API{Base: server.URL, Namespace: "team-commerce", HTTP: server.Client()}, f
}
func cloneResource(r k.Resource) k.Resource {
	b, _ := json.Marshal(r)
	var v k.Resource
	_ = json.Unmarshal(b, &v)
	return v
}
func (f *kubeFixture) key(kind, name string) string { return kind + "/" + name }
func (f *kubeFixture) writeCount() int              { f.mu.Lock(); defer f.mu.Unlock(); return f.writes }
func (f *kubeFixture) put(r k.Resource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[f.key(r.Kind, r.Metadata.Name)] = cloneResource(r)
}
func (f *kubeFixture) get(kind, name string) k.Resource {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneResource(f.objects[f.key(kind, name)])
}
func (f *kubeFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := "/apis/nines.dev/v1alpha1/namespaces/team-commerce/"
	monitoring := "/apis/monitoring.coreos.com/v1/namespaces/team-commerce/"
	var path string
	if strings.HasPrefix(r.URL.Path, prefix) {
		path = strings.TrimPrefix(r.URL.Path, prefix)
	} else if strings.HasPrefix(r.URL.Path, monitoring) {
		path = strings.TrimPrefix(r.URL.Path, monitoring)
	} else {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	kind := ""
	for k, p := range Plurals {
		if p == parts[0] {
			kind = k
		}
	}
	if parts[0] == "prometheusrules" {
		kind = "PrometheusRule"
	}
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 && r.Method == "GET" {
		items := []k.Resource{}
		for _, v := range f.objects {
			if v.Kind == kind {
				items = append(items, v)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}
	name := ""
	if len(parts) > 1 {
		name = parts[1]
	}
	key := f.key(kind, name)
	current, exists := f.objects[key]
	if r.Method == "GET" {
		if !exists {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(current)
		return
	}
	if r.Method == http.MethodDelete {
		var options struct {
			Preconditions struct {
				UID             string `json:"uid"`
				ResourceVersion string `json:"resourceVersion"`
			} `json:"preconditions"`
		}
		if json.NewDecoder(r.Body).Decode(&options) != nil || !exists || options.Preconditions.UID != current.Metadata.UID || options.Preconditions.ResourceVersion != current.Metadata.ResourceVersion {
			w.WriteHeader(http.StatusConflict)
			return
		}
		f.writes++
		if len(current.Metadata.Finalizers) > 0 {
			now := time.Now().UTC()
			current.Metadata.DeletionTimestamp = &now
			f.objects[key] = current
		} else {
			delete(f.objects, key)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	var incoming k.Resource
	if json.NewDecoder(r.Body).Decode(&incoming) != nil {
		http.Error(w, "bad JSON", 400)
		return
	}
	f.writes++
	switch r.Method {
	case "POST":
		key = f.key(kind, incoming.Metadata.Name)
		if _, exists = f.objects[key]; exists {
			w.WriteHeader(409)
			return
		}
		created := time.Now().UTC()
		incoming.Metadata.CreationTimestamp = &created
		incoming.Metadata.UID = model.NewID()
		incoming.Metadata.Generation = 1
	case "PUT":
		if !exists {
			http.NotFound(w, r)
			return
		}
		if incoming.Metadata.ResourceVersion != current.Metadata.ResourceVersion {
			w.WriteHeader(409)
			return
		}
		if current.Metadata.CreationTimestamp != nil && (incoming.Metadata.CreationTimestamp == nil || !incoming.Metadata.CreationTimestamp.Equal(*current.Metadata.CreationTimestamp)) {
			http.Error(w, "creationTimestamp is immutable", http.StatusUnprocessableEntity)
			return
		}

		if len(parts) > 2 && parts[2] == "status" {
			current.Status = incoming.Status
			incoming = current
		} else {
			incoming.Metadata.Generation = current.Metadata.Generation
			if CanonicalDigest(incoming.Spec) != CanonicalDigest(current.Spec) {
				incoming.Metadata.Generation++
			}
		}
	default:
		w.WriteHeader(405)
		return
	}
	f.next++
	incoming.Metadata.ResourceVersion = fmt.Sprint(f.next)
	f.objects[key] = cloneResource(incoming)
	_ = json.NewEncoder(w).Encode(incoming)
}
func controller(t *testing.T) (*Controller, *kubeFixture) {
	api, f := newKube(t)
	backend := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backend.Close)
	return &Controller{API: api, Config: Config{Namespace: "team-commerce", Tenant: "commerce", Projects: []string{"commerce"}, Sources: map[string]Source{"primary": {Kind: "prometheus-operator", QueryURL: backend.URL, PrometheusLabels: map[string]string{"prometheus": "primary"}}}}}, f
}
func sloResource(t *testing.T) k.Resource {
	t.Helper()
	d := definition()
	r, e := FromDefinition(model.Definition{SLO: &d}, "team-commerce")
	if e != nil {
		t.Fatal(e)
	}
	r.Metadata.UID = "slo-uid"
	r.Metadata.Generation = 1
	r.Metadata.ResourceVersion = "initial"
	return r
}
func TestPrometheusReconciliationOwnershipIdempotencyAndScope(t *testing.T) {
	c, f := controller(t)
	ctx := context.Background()
	r := sloResource(t)
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(v1.SLOKind, r.Metadata.Name)
	if r.Status.Conditions[0].Reason != "RulesConfigured" {
		t.Fatal(r.Status)
	}
	before := f.writeCount()
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	if f.writeCount() != before {
		t.Fatal("unchanged reconciliation wrote again")
	}
	rule := f.get("PrometheusRule", RuleName(r.Metadata.Name))
	rule.Metadata.OwnerReferences[0].UID = "someone-else"
	f.put(rule)
	if e := c.Reconcile(ctx, r); e != ErrOwnership {
		t.Fatal("foreign rule overwritten", e)
	}
	r.Metadata.Namespace = "another-team"
	before = f.writeCount()
	if e := c.Reconcile(ctx, r); e == nil || f.writeCount() != before {
		t.Fatal("cross-namespace write", e)
	}
	r.Metadata.Namespace = c.Config.Namespace
	r = f.get(v1.SLOKind, r.Metadata.Name)
	r.Metadata.Annotations[k.ProjectAnnotation] = "forbidden"
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	if f.get(v1.SLOKind, r.Metadata.Name).Status.Conditions[0].Reason != "ScopeDenied" {
		t.Fatal("missing scope condition")
	}
}
func TestMimirPreparedDestinationAcceptanceDeletionAndUIDFence(t *testing.T) {
	c, f := controller(t)
	ctx := context.Background()
	var group rules.Group
	posts, deletes := 0, 0
	ruler := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Scope-OrgID") != "commerce-dataset" {
			t.Error("wrong dataset")
		}
		switch r.Method {
		case "GET":
			if len(group.Rules) == 0 {
				http.NotFound(w, r)
				return
			}
			b, _ := yaml.Marshal(group)
			_, _ = w.Write(b)
		case "POST":
			posts++
			if e := yaml.NewDecoder(r.Body).Decode(&group); e != nil {
				t.Error(e)
			}
			w.WriteHeader(202)
		case "DELETE":
			deletes++
			group = rules.Group{}
			w.WriteHeader(202)
		}
	}))
	defer ruler.Close()
	c.Config.Sources["primary"] = Source{Kind: "mimir", QueryURL: ruler.URL + "/prometheus", RulerURL: ruler.URL + "/prometheus", MimirOrgID: "commerce-dataset", RulerNamespace: "nines-commerce"}
	r := sloResource(t)
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	if len(r.Metadata.Finalizers) != 1 || posts != 0 {
		t.Fatal("write before cleanup was installed")
	}
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	if r.Status.ManagedGroup == "" || posts != 0 {
		t.Fatal("destination intent was not durable first")
	}
	c.API.BeforeWrite = func() error { return fmt.Errorf("lease lost") }
	if e := c.Reconcile(ctx, r); e == nil || posts != 0 {
		t.Fatal("expired lease published rules", e)
	}
	c.API.BeforeWrite = nil

	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	if posts != 1 || r.Status.Conditions[0].Reason != "RulesAccepted" {
		t.Fatal(posts, r.Status)
	}
	if e := c.Reconcile(ctx, r); e != nil || posts != 1 {
		t.Fatal("duplicate publish", e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	changed := c.Config.Sources["primary"]
	original := changed
	changed.MimirOrgID = "another-dataset"
	c.Config.Sources["primary"] = changed
	if e := c.Reconcile(ctx, r); e == nil || posts != 1 {
		t.Fatal("destination changed without cleanup", e)
	}
	c.Config.Sources["primary"] = original
	r = f.get(r.Kind, r.Metadata.Name)
	now := time.Now()
	r.Metadata.DeletionTimestamp = &now
	f.put(r)
	// A recreated resource cannot remove the earlier resource's group.
	c2 := *c
	different := r
	different.Metadata.UID = "new-uid"
	if e := c2.cleanup(ctx, different); e == nil || deletes != 0 {
		t.Fatal("foreign UID cleanup", e)
	}
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	if deletes != 1 || len(r.Metadata.Finalizers) != 0 {
		t.Fatal("owned group was not cleaned", deletes, r.Metadata.Finalizers)
	}
}
func TestIntentCompilesUpdatedBindingGenerationWithLiveEvidence(t *testing.T) {
	c, f := controller(t)
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
	c.Config.Projects = []string{"test"}
	c.Config.Sources["primary"] = Source{Kind: "prometheus-operator", QueryURL: telemetry.URL}
	b := testtelemetry.Binding()
	br, e := FromDefinition(model.Definition{Binding: &b}, c.Config.Namespace)
	if e != nil {
		t.Fatal(e)
	}
	br.Metadata.UID = "binding-uid"
	br.Metadata.Generation = 3
	br.Metadata.ResourceVersion = "binding"
	f.put(br)
	policies := []v1.BurnRatePolicy{{Name: "fast", Severity: "critical", ShortWindowSeconds: 60, LongWindowSeconds: 300, BurnRate: 14.4, MinCoverage: 1}, {Name: "sustained", Severity: "warning", ShortWindowSeconds: 300, LongWindowSeconds: 1800, BurnRate: 6, MinCoverage: 1}}
	spec := k.IntentSpec{IntentSpec: v1.IntentSpec{Description: "99.9% of eligible attempts succeed", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown", Policies: policies}, MeasurementSelection: k.MeasurementSelection{Name: br.Metadata.Name, ExpectedGeneration: 3}}
	raw, _ := json.Marshal(spec)
	r := k.Resource{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: k.Metadata{Name: "intent-slo", Namespace: c.Config.Namespace, UID: "intent-uid", Generation: 1, ResourceVersion: "intent", Annotations: map[string]string{k.ProjectAnnotation: "test"}}, Spec: raw}
	f.put(r)
	if e = c.Reconcile(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	child := f.get(v1.SLOKind, r.Metadata.Name)
	d, e := Native(child)
	if e != nil || d.SLO.Spec.MeasurementBinding.Revision != 3 || !owned(child, "intent-uid") {
		t.Fatal(child, e)
	}
	if model.DefinitionDigest(d.SLO.Spec.Policies) != model.DefinitionDigest(policies) {
		t.Fatal("intent compilation dropped named severity policies", d.SLO.Spec.Policies)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	if r.Status.Conditions[0].Reason != "IntentCompiled" {
		t.Fatal(r.Status)
	}
	// A later generation requires another explicit selection.
	br.Metadata.Generation = 4
	f.put(br)
	if e = c.Reconcile(context.Background(), r); e == nil {
		t.Fatal("binding changed without confirmation")
	}
}
func TestConfigSecretsAndPaginationFailClosed(t *testing.T) {
	c, _ := controller(t)
	if e := c.Config.Validate(); e != nil {
		t.Fatal(e)
	}
	source := Source{Kind: "mimir", QueryURL: "http://127.0.0.1:9000/prometheus", RulerURL: "http://127.0.0.1:9000/prometheus", MimirOrgID: "one|two", RulerNamespace: "nines"}
	c.Config.Sources["primary"] = source
	if c.Config.Validate() == nil {
		t.Fatal("federated tenant accepted")
	}
	path := filepath.Join(t.TempDir(), "token")
	if e := os.WriteFile(path, []byte("private-token"), 0600); e != nil {
		t.Fatal(e)
	}
	source.BearerTokenFile = path
	b, _ := json.Marshal(source)
	if strings.Contains(string(b), path) {
		t.Fatal("credential path exported")
	}
	api, _ := newKube(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "metadata": map[string]string{"continue": "forever"}})
	}))
	defer server.Close()
	api.Base = server.URL
	if _, e := api.List(context.Background(), v1.SLOKind); e == nil {
		t.Fatal("incomplete pagination accepted")
	}
	// Kubernetes credentials must never follow an API redirect.
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
	defer redirected.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, redirected.URL, http.StatusFound) }))
	defer redirect.Close()
	api.Base = redirect.URL
	api.TokenFile = path
	if _, e := api.List(context.Background(), v1.SLOKind); !code(e, 302) {
		t.Fatal("redirect was not rejected", e)
	}
}

func TestNamespaceLeaseExcludesAnotherControllerAndAllowsExpiry(t *testing.T) {
	var mu sync.Mutex
	var saved lease
	exists := false
	revision := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/apis/coordination.k8s.io/v1/namespaces/team-commerce/leases") {
			http.NotFound(w, r)
			return
		}
		if r.Method == "GET" {
			if !exists {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(saved)
			return
		}
		var incoming lease
		if json.NewDecoder(r.Body).Decode(&incoming) != nil {
			w.WriteHeader(400)
			return
		}
		if r.Method == "POST" && exists || r.Method == "PUT" && (!exists || incoming.Metadata.ResourceVersion != saved.Metadata.ResourceVersion) {
			w.WriteHeader(409)
			return
		}
		revision++
		incoming.Metadata.ResourceVersion = fmt.Sprint(revision)
		saved = incoming
		exists = true
		_ = json.NewEncoder(w).Encode(saved)
	}))
	defer server.Close()
	api := &API{Base: server.URL, Namespace: "team-commerce", HTTP: server.Client()}
	ctx := context.Background()
	if e := api.renew(ctx, "first"); e != nil {
		t.Fatal(e)
	}
	if e := api.renew(ctx, "second"); e == nil {
		t.Fatal("concurrent leader acquired lease")
	}
	if e := api.renew(ctx, "first"); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	saved.Spec.RenewTime = leaseTime{Time: time.Now().Add(-31 * time.Second)}
	mu.Unlock()
	if e := api.renew(ctx, "second"); e != nil {
		t.Fatal(e)
	}
	if e := api.renew(ctx, "first"); e == nil {
		t.Fatal("old holder retained authority")
	}
}

func TestMeasurementEditsUseNewRuleRevisionAndAlertIdentity(t *testing.T) {
	c, f := controller(t)
	ctx := context.Background()
	r := sloResource(t)
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	var spec v1.SLOSpec
	if e := json.Unmarshal(r.Spec, &spec); e != nil {
		t.Fatal(e)
	}
	spec.Target = .998
	r.Spec, _ = json.Marshal(spec)
	r.Metadata.Generation = 2
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal(e)
	}
	r = f.get(r.Kind, r.Metadata.Name)
	rule := f.get("PrometheusRule", RuleName(r.Metadata.Name))
	var file rules.File
	if e := json.Unmarshal(rule.Spec, &file); e != nil {
		t.Fatal(e)
	}
	alert := false
	for _, generated := range file.Groups[0].Rules {
		if generated.Alert != "" {
			alert = true
			if generated.Labels["nines_revision"] != "2" {
				t.Fatal("alert timer identity did not advance", generated.Labels)
			}
		}
	}
	if !alert {
		t.Fatal("missing burn alert")
	}
	before := CanonicalDigest(rule.Spec)
	spec.GoodQuery = "sum(increase(other_requests_total[1m]))"
	r.Spec, _ = json.Marshal(spec)
	r.Metadata.Generation = 3
	// A lost status write still uses the new generation's revision labels.
	r.Status = nil
	f.put(r)
	if e := c.Reconcile(ctx, r); e != nil {
		t.Fatal("measurement edit failed", e)
	}
	updated := f.get("PrometheusRule", RuleName(r.Metadata.Name))
	if CanonicalDigest(updated.Spec) == before {
		t.Fatal("measurement rules unchanged")
	}
	if e := json.Unmarshal(updated.Spec, &file); e != nil {
		t.Fatal(e)
	}
	for _, generated := range file.Groups[0].Rules {
		if generated.Labels["nines_revision"] != "3" {
			t.Fatal("old history reused", generated.Labels)
		}
	}
	if !strings.Contains(string(updated.Spec), "other_requests_total") {
		t.Fatal("corrected query missing")
	}
}

func TestOwnedUpdatesPreserveKubernetesMetadataAndOtherOwners(t *testing.T) {
	api, f := newKube(t)
	ctx := context.Background()
	d := definition()
	desired, e := PrometheusRule(d, "commerce", api.Namespace, "root-uid", nil, 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ApplyOwned(ctx, desired, "root-uid"); e != nil {
		t.Fatal(e)
	}
	current := f.get(desired.Kind, desired.Metadata.Name)
	block := true
	current.Metadata.OwnerReferences = append(current.Metadata.OwnerReferences, k.OwnerReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "external", UID: "external-uid", BlockOwnerDeletion: &block})
	current.Metadata.Annotations["other-owner"] = "retain"
	f.put(current)
	d.Spec.Target = .998
	next, e := PrometheusRule(d, "commerce", api.Namespace, "root-uid", nil, 2)
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ApplyOwned(ctx, next, "root-uid"); e != nil {
		t.Fatal(e)
	}
	updated := f.get(next.Kind, next.Metadata.Name)
	if updated.Metadata.CreationTimestamp == nil || len(updated.Metadata.OwnerReferences) != 2 || updated.Metadata.OwnerReferences[1].BlockOwnerDeletion == nil || !*updated.Metadata.OwnerReferences[1].BlockOwnerDeletion || updated.Metadata.Annotations["other-owner"] != "retain" {
		t.Fatal("Kubernetes metadata was dropped", updated.Metadata)
	}
}

func TestRenderRejectsKindsWithoutOperatorCRD(t *testing.T) {
	graph := testtelemetry.Graph(false)
	if _, err := FromDefinition(model.Definition{DependencyMap: &graph}, "team-commerce"); err == nil {
		t.Fatal("unsupported CRD kind was emitted")
	}
}

// Keep Kubernetes admission from silently pruning fields supported by the API.
func TestCheckedInCRDsMatchSharedSchema(t *testing.T) {
	var want bytes.Buffer
	for i, crd := range CRDs() {
		if i > 0 {
			want.WriteString("---\n")
		}
		body, err := YAML(crd)
		if err != nil {
			t.Fatal(err)
		}
		want.Write(body)
	}
	got, err := os.ReadFile("../../deploy/operator/crds.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatal("checked-in CRDs differ from shared API types; regenerate with nines operator crds --offline")
	}
}

func TestInlineIntentOperatorNeedsNoBindingCR(t *testing.T) {
	c, f := controller(t)
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
	c.Config.Projects = []string{"test"}
	c.Config.Sources["primary"] = Source{Kind: "prometheus-operator", QueryURL: telemetry.URL}
	goal := v1.SLOIntent{APIVersion: v1.APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "inline-operator", Project: "test"}, Spec: v1.IntentSpec{Description: "API requests should succeed", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, StepSeconds: 60, NoDataPolicy: "unknown", Measurement: &v1.MeasurementSpec{Type: "availability", Metric: "http_requests_total", Labels: map[string]string{"service": "api"}, SuccessLabels: map[string]string{"status": "200"}, ScrapeIntervalSeconds: 10, ObservationPoint: "API completions"}}}
	r, e := FromIntent(goal, c.Config.Namespace, k.MeasurementSelection{})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(r.Spec), "measurementSelection") {
		t.Fatal("emitted a phantom binding", string(r.Spec))
	}
	r.Metadata.UID = "inline-intent-uid"
	r.Metadata.Generation = 1
	r.Metadata.ResourceVersion = "inline-intent"
	f.put(r)
	if e = c.Reconcile(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	child := f.get(v1.SLOKind, r.Metadata.Name)
	d, e := Native(child)
	if e != nil || d.SLO.Spec.Measurement == nil || d.SLO.Spec.MeasurementBinding != nil || !owned(child, r.Metadata.UID) {
		t.Fatal(child, e)
	}
	if e = c.Reconcile(context.Background(), child); e != nil {
		t.Fatal(e)
	}
	rule := f.get("PrometheusRule", RuleName(r.Metadata.Name))
	if !owned(rule, child.Metadata.UID) {
		t.Fatal("rules not owned by inline SLO", rule)
	}
}
