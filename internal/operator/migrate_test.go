package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	k "github.com/ishantanu/nines/api/kubernetes/v1alpha1"
	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// migrationCluster is a fake API server holding objects as they were last
// written, with transitional or final CRDs.
type migrationCluster struct {
	mu      sync.Mutex
	crds    map[string]map[string]any // plural -> CRD object
	objects map[string]k.Resource     // plural/namespace/name -> as last written
	writes  int
}

func (m *migrationCluster) serve(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name, ok := strings.CutPrefix(r.URL.Path, "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/"); ok {
		name, status := strings.CutSuffix(name, "/status")
		plural, _, _ := strings.Cut(name, ".")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(m.crds[plural])
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !status {
			http.Error(w, "status only", 400)
			return
		}
		m.crds[plural]["status"] = body["status"]
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/apis/nines.dev/")
	version, rest, _ := strings.Cut(rest, "/")
	if r.Method == "GET" {
		items := []k.Resource{}
		for key, o := range m.objects {
			if strings.HasPrefix(key, rest+"/") {
				o.APIVersion = "nines.dev/" + version
				items = append(items, o)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}
	parts := strings.Split(rest, "/") // namespaces/NS/PLURAL/NAME
	var o k.Resource
	_ = json.NewDecoder(r.Body).Decode(&o)
	o.Metadata.ManagedFields = []json.RawMessage{json.RawMessage(`{"apiVersion":"nines.dev/` + version + `","time":"2026-11-23T00:00:00Z"}`)}
	m.objects[parts[2]+"/"+parts[1]+"/"+parts[3]] = o
	m.writes++
}

func crdWith(plural string, versions ...map[string]any) map[string]any {
	return map[string]any{"metadata": map[string]any{"name": plural + ".nines.dev"}, "spec": map[string]any{"versions": versions}, "status": map[string]any{"storedVersions": []string{"v1alpha1"}}}
}

func TestMigrateRewritesStoredV1alpha1Resources(t *testing.T) {
	m := &migrationCluster{crds: map[string]map[string]any{}, objects: map[string]k.Resource{}}
	server := httptest.NewServer(http.HandlerFunc(m.serve))
	defer server.Close()
	api := &API{Base: server.URL, HTTP: server.Client()}
	final := func(plural string) map[string]any {
		return crdWith(plural, map[string]any{"name": "v1beta1", "served": true, "storage": true})
	}
	for _, kind := range migrated {
		m.crds[Plurals[kind]] = final(Plurals[kind])
	}
	// Without the transitional CRDs nothing is touched.
	if err := Migrate(context.Background(), api, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "crds-transitional.yaml") {
		t.Fatal("migrated without transitional CRDs", err)
	}
	for _, kind := range migrated {
		m.crds[Plurals[kind]] = crdWith(Plurals[kind], map[string]any{"name": "v1beta1", "served": true, "storage": true}, map[string]any{"name": "v1alpha1", "served": true, "storage": false})
	}
	old := func(file string) k.Resource {
		raw, err := os.ReadFile("../../api/v1beta1/testdata/v1alpha1/" + file)
		if err != nil {
			t.Fatal(err)
		}
		d, err := model.ParseDefinition(raw)
		if err != nil {
			t.Fatal(err)
		}
		r, err := legacyResource(d, "team-commerce")
		if err != nil {
			t.Fatal(err)
		}
		r.Metadata.ManagedFields = []json.RawMessage{json.RawMessage(`{"apiVersion":"nines.dev/v1alpha1","time":"2026-01-01T00:00:00Z"}`)}
		return r
	}
	slo, journey, binding := old("request-success.slo.yaml"), old("checkout.journey.yaml"), old("request-success.binding.yaml")
	m.objects["servicelevelobjectives/team-commerce/"+slo.Metadata.Name] = slo
	m.objects["criticaluserjourneys/team-commerce/"+journey.Metadata.Name] = journey
	m.objects["measurementbindings/team-commerce/"+binding.Metadata.Name] = binding
	var log bytes.Buffer
	if err := Migrate(context.Background(), api, &log); err != nil {
		t.Fatal(err, log.String())
	}
	// Each migrated object stores exactly the original definition.
	for key, file := range map[string]string{
		"servicelevelobjectives/team-commerce/" + slo.Metadata.Name:   "request-success.slo.yaml",
		"criticaluserjourneys/team-commerce/" + journey.Metadata.Name: "checkout.journey.yaml",
		"measurementbindings/team-commerce/" + binding.Metadata.Name:  "request-success.binding.yaml",
	} {
		o := m.objects[key]
		if o.APIVersion != "nines.dev/v1beta1" {
			t.Fatal(key, "not written as v1beta1", o.APIVersion)
		}
		got, err := Native(o)
		if err != nil {
			t.Fatal(key, err)
		}
		original, _ := model.ParseDefinition(mustRead(t, "../../api/v1beta1/testdata/v1alpha1/"+file))
		if model.DefinitionDigest(got.Manifest()) != model.DefinitionDigest(original.Manifest()) {
			t.Fatal(key, "changed in migration")
		}
	}
	for _, kind := range migrated {
		stored := m.crds[Plurals[kind]]["status"].(map[string]any)["storedVersions"]
		if b, _ := json.Marshal(stored); string(b) != `["v1beta1"]` {
			t.Fatal(kind, "stored versions not updated", string(b))
		}
	}
	if !strings.Contains(log.String(), `"name":"request-success"`) {
		t.Fatal("original objects were not logged as a backup")
	}
	// Running it again changes nothing.
	writes := m.writes
	if err := Migrate(context.Background(), api, &bytes.Buffer{}); err != nil || m.writes != writes {
		t.Fatal("second migration wrote again", err)
	}
}

func TestMigrateSkipsResourcesAlreadyWrittenAsV1beta1(t *testing.T) {
	r := sloResource(t)
	r.Metadata.ManagedFields = []json.RawMessage{
		json.RawMessage(`{"apiVersion":"nines.dev/v1alpha1","time":"2026-01-01T00:00:00Z"}`),
		json.RawMessage(`{"apiVersion":"nines.dev/v1beta1","time":"2026-02-01T00:00:00Z"}`),
		json.RawMessage(`{"apiVersion":"nines.dev/v1alpha1","time":"2026-03-01T00:00:00Z","subresource":"status"}`),
	}
	if lastWrite(r) != "nines.dev/v1beta1" {
		t.Fatal("status writes decided the stored version")
	}
}

func TestConvertDocumentsForGitOps(t *testing.T) {
	out, err := ConvertDocuments(mustRead(t, "../../examples/operator/compiled.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "nines.dev/v1alpha1") || strings.Contains(string(out), "Seconds") {
		t.Fatal("v1alpha1 left after conversion", string(out))
	}
	again, err := ConvertDocuments(out)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatal("conversion is not idempotent", err)
	}
}

// A v1beta1 journey's omitted dimension follows the linked SLO resource.
func TestOperatorResolvesJourneyDimensionsFromLinkedSLOs(t *testing.T) {
	c, f := controller(t)
	d := definition()
	d.Spec.Measurement = &v1.MeasurementSpec{Type: "latency", Metric: "http_request_duration_seconds", Labels: map[string]string{"service": "checkout"}, ThresholdSeconds: func() *float64 { v := 0.3; return &v }(), ScrapeIntervalSeconds: 15, ObservationPoint: "Server."}
	d.Spec.GoodQuery, d.Spec.TotalQuery = "", ""
	slo, err := FromDefinition(model.Definition{SLO: &d}, "team-commerce")
	if err != nil {
		t.Fatal(err)
	}
	f.put(slo)
	journey := k.Resource{APIVersion: "nines.dev/v1beta1", Kind: v1.JourneyKind, Metadata: k.Metadata{Name: "buy", Namespace: "team-commerce", Annotations: map[string]string{k.ProjectAnnotation: "commerce"}}, Spec: json.RawMessage(`{"displayName":"Buy","description":"Buy.","owner":"team","criticality":"high","slos":[{"id":"checkout","required":true}],"steps":[{"id":"pay","name":"Pay","actor":"user","sloRefs":["checkout"]}]}`)}
	got, err := c.native(context.Background(), journey)
	if err != nil || got.Journey.Spec.SLOs[0].Dimension != "latency" {
		t.Fatal("dimension not taken from the latency SLO", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// legacyResource is how the v1alpha1 operator wrote a definition.
func legacyResource(d model.Definition, namespace string) (k.Resource, error) {
	b, err := v1.CanonicalDefinitionJSON(d.Manifest())
	if err != nil {
		return k.Resource{}, err
	}
	var obj struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err = json.Unmarshal(b, &obj); err != nil {
		return k.Resource{}, err
	}
	return k.Resource{APIVersion: v1.APIVersion, Kind: d.Kind(), Metadata: k.Metadata{Name: d.Metadata().Name, Namespace: namespace, Annotations: map[string]string{k.ProjectAnnotation: d.Metadata().Project}}, Spec: obj.Spec}, nil
}

// The migration Job must run the release the operator is upgraded to.
func TestMigrationJobUsesTheOperatorImage(t *testing.T) {
	image := func(file string) string {
		for _, line := range strings.Split(string(mustRead(t, "../../deploy/operator/"+file)), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "image: "); ok {
				return v
			}
		}
		t.Fatal("no image in", file)
		return ""
	}
	if image("migrate.yaml") != image("operator.yaml") {
		t.Fatal("migrate.yaml and operator.yaml use different images")
	}
}

// Deprecated CRD versions are marked so the API server warns kubectl users.
func TestDeprecatedCRDVersionsAreMarked(t *testing.T) {
	check := func(crds []any, want int) {
		t.Helper()
		marked := 0
		for _, crd := range crds {
			for _, v := range crd.(map[string]any)["spec"].(map[string]any)["versions"].([]map[string]any) {
				if v["deprecated"] == true {
					marked++
					if v["name"] != "v1alpha1" || v["deprecationWarning"] != model.V1alpha1Deprecation {
						t.Fatal("unexpected deprecation", v["name"], v["deprecationWarning"])
					}
				}
			}
		}
		if marked != want {
			t.Fatalf("%d deprecated versions, want %d", marked, want)
		}
	}
	check(CRDs(), 0)
	check(TransitionalCRDs(), 3)
}
