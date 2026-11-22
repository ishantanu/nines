package v1beta1

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

// The portal converts definitions in the browser. These fixtures record what
// the Go conversion produces, and web/src/lib/v1beta1.test.ts requires the
// TypeScript conversion to produce exactly the same. Run with
// UPDATE_V1BETA1_FIXTURES=1 to rewrite them after changing the Go rules.
type exportCase struct {
	Name string                         `json:"name"`
	Hub  v1alpha1.ServiceLevelObjective `json:"hub"`
	Beta ServiceLevelObjective          `json:"beta"`
}
type importCase struct {
	Name  string                          `json:"name"`
	Beta  ServiceLevelObjective           `json:"beta"`
	Hub   *v1alpha1.ServiceLevelObjective `json:"hub,omitempty"`
	Notes []string                        `json:"notes"`
	Error string                          `json:"error,omitempty"`
}

// Journey cases record the linked SLOs' measurement types ("" for an SLO
// without one) so the TypeScript side can build the same lookup.
type journeyCase struct {
	Name  string                        `json:"name"`
	SLOs  map[string]string             `json:"slos"`
	Hub   *v1alpha1.CriticalUserJourney `json:"hub,omitempty"`
	Beta  CriticalUserJourney           `json:"beta"`
	Notes []string                      `json:"notes"`
	Error string                        `json:"error,omitempty"`
}
type bindingCase struct {
	Name string                      `json:"name"`
	Hub  v1alpha1.MeasurementBinding `json:"hub"`
	Beta MeasurementBinding          `json:"beta"`
}
type mapCase struct {
	Name string                        `json:"name"`
	Hub  v1alpha1.ServiceDependencyMap `json:"hub"`
	Beta ServiceDependencyMap          `json:"beta"`
}
type fixtures struct {
	Export         []exportCase  `json:"export"`
	Import         []importCase  `json:"import"`
	JourneyExport  []journeyCase `json:"journeyExport"`
	JourneyImport  []journeyCase `json:"journeyImport"`
	Bindings       []bindingCase `json:"bindings"`
	DependencyMaps []mapCase     `json:"dependencyMaps"`
}

func typedLookup(slos map[string]string) SLOLookup {
	return func(id string) (v1alpha1.SLOSpec, bool) {
		kind, ok := slos[id]
		if !ok {
			return v1alpha1.SLOSpec{}, false
		}
		if kind == "" {
			return v1alpha1.SLOSpec{}, true
		}
		return v1alpha1.SLOSpec{Measurement: &v1alpha1.MeasurementSpec{Type: kind}}, true
	}
}

func addTopologyFixtures(t *testing.T, out *fixtures) {
	t.Helper()
	for name, raw := range shipped(t, JourneyKind) {
		var hub v1alpha1.CriticalUserJourney
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(err)
		}
		slos := map[string]string{}
		for _, l := range hub.Spec.SLOs {
			slos[l.ID] = ""
			if l.Dimension == "latency" {
				slos[l.ID] = "latency"
			}
		}
		h := hub
		out.JourneyExport = append(out.JourneyExport, journeyCase{Name: name, SLOs: slos, Hub: &h, Beta: JourneyFromHub(hub, typedLookup(slos)), Notes: []string{}})
	}
	links := func(l ...JourneySLO) CriticalUserJourney {
		refs := []string{}
		for _, x := range l {
			refs = append(refs, x.ID)
		}
		return CriticalUserJourney{APIVersion: APIVersion, Kind: JourneyKind, Metadata: Metadata{Name: "checkout", Project: "p"}, Spec: JourneySpec{DisplayName: "Checkout", Description: "Buy.", Owner: "team", Criticality: "high", SLOs: l, Steps: []JourneyStep{{ID: "pay", Name: "Pay", Actor: "user", SLORefs: refs}}, RunObjective: &RunObjective{Target: 0.9, Window: D(86400e9), MinRuns: 5}, Agentic: &AgenticContract{SuccessDefinition: "Paid.", MaxDuration: ptr(D(120e9)), MaxSteps: 4}}}
	}
	slos := map[string]string{"api": "", "search": "latency", "store": "durability"}
	for _, c := range []struct {
		name string
		beta CriticalUserJourney
		slos map[string]string
	}{
		{"derived dimensions", links(JourneySLO{ID: "api", Required: true}, JourneySLO{ID: "search"}, JourneySLO{ID: "store"}, JourneySLO{ID: "spend", Dimension: "cost"}), slos},
		{"contradiction", links(JourneySLO{ID: "search", Dimension: "availability"}), slos},
		{"untyped explicit", links(JourneySLO{ID: "api", Dimension: "correctness"}), slos},
		{"unknown SLO", links(JourneySLO{ID: "missing"}), slos},
	} {
		hub, notes, err := c.beta.ToHub(typedLookup(c.slos))
		jc := journeyCase{Name: c.name, SLOs: c.slos, Beta: c.beta, Notes: notes}
		if notes == nil {
			jc.Notes = []string{}
		}
		if err != nil {
			jc.Error = err.Error()
		} else {
			jc.Hub = &hub
		}
		out.JourneyImport = append(out.JourneyImport, jc)
	}
	for name, raw := range shipped(t, BindingKind) {
		var hub v1alpha1.MeasurementBinding
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(err)
		}
		out.Bindings = append(out.Bindings, bindingCase{name, hub, BindingFromHub(hub)})
	}
	observed := v1alpha1.ServiceDependencyMap{APIVersion: v1alpha1.APIVersion, Kind: DependencyMapKind, Metadata: Metadata{Name: "commerce", Project: "p"}, Spec: v1alpha1.DependencyMapSpec{DisplayName: "Commerce", Services: []v1alpha1.MapService{{Name: "api", DisplayName: "API", SLORefs: []string{"api"}, Position: &v1alpha1.GraphPosition{X: 10, Y: 20}}}, Dependencies: []v1alpha1.DeclaredDependency{{Caller: "api", Callee: "db", Critical: true}}, Observed: &v1alpha1.GraphObservation{Source: "mimir", Labels: map[string]string{"cluster": "a"}, WindowSeconds: 3600, ScrapeIntervalSeconds: 30, MaxSampleAgeSeconds: 120}}}
	out.DependencyMaps = append(out.DependencyMaps, mapCase{"observed", observed, DependencyMapFromHub(observed)})
	for name, raw := range shipped(t, DependencyMapKind) {
		var hub v1alpha1.ServiceDependencyMap
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(err)
		}
		out.DependencyMaps = append(out.DependencyMaps, mapCase{name, hub, DependencyMapFromHub(hub)})
	}
	sort.Slice(out.JourneyExport, func(i, j int) bool { return out.JourneyExport[i].Name < out.JourneyExport[j].Name })
	sort.Slice(out.Bindings, func(i, j int) bool { return out.Bindings[i].Name < out.Bindings[j].Name })
	sort.Slice(out.DependencyMaps, func(i, j int) bool { return out.DependencyMaps[i].Name < out.DependencyMaps[j].Name })
}

func ptr[T any](v T) *T { return &v }

func buildFixtures(t *testing.T) fixtures {
	t.Helper()
	var out fixtures
	files, _ := filepath.Glob("../../examples/*.slo.yaml")
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		var hub v1alpha1.ServiceLevelObjective
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(err)
		}
		out.Export = append(out.Export, exportCase{filepath.Base(f), hub, FromHub(hub)})
	}
	pushed := v1alpha1.NewSLO("pushed", "p", v1alpha1.SLOSpec{DisplayName: "Pushed", Service: "s", Target: 0.99, WindowSeconds: 86400, StepSeconds: 60})
	out.Export = append(out.Export, exportCase{"pushed samples, no policies", pushed, FromHub(pushed)})
	window := v1alpha1.NewSLO("window", "p", v1alpha1.SLOSpec{DisplayName: "Window", Service: "s", Source: "mimir", Target: 0.995, WindowSeconds: 7 * 86400, StepSeconds: 300, ProviderWindow: &v1alpha1.ProviderWindow{TotalSelector: "requests_total", BadSelector: "errors_total", ScrapeIntervalSeconds: 30}, Policies: []v1alpha1.BurnRatePolicy{{Name: "page", Severity: "critical", ShortWindowSeconds: 600, LongWindowSeconds: 3600, BurnRate: 10, ForSeconds: 120, MinCoverage: 0.9, MinEvents: 50, Confidence: 0.95}}, BadInterval: &v1alpha1.BadIntervalPolicy{ErrorRate: 0.05, MinEvents: 10}, Quality: &v1alpha1.MeasurementQualityPolicy{MaxSampleAgeSeconds: 600, CohortLabels: []string{"region"}}, Lifecycle: v1alpha1.LifecycleExperimental, Labels: map[string]string{"team": "core"}})
	out.Export = append(out.Export, exportCase{"window selectors and options", window, FromHub(window)})

	base := func(name string) ServiceLevelObjective {
		return ServiceLevelObjective{APIVersion: APIVersion, Kind: SLOKind, Metadata: Metadata{Name: name, Project: "p"}, Spec: SLOSpec{DisplayName: "API", Service: "api", Target: 0.999, Queries: &Queries{Good: "sum(good)", Total: "sum(total)"}}}
	}
	day := D(86400e9)
	cases := map[string]func(*ServiceLevelObjective){
		"defaults":       func(*ServiceLevelObjective) {},
		"one day window": func(s *ServiceLevelObjective) { s.Spec.Window = &day },
		"loose target":   func(s *ServiceLevelObjective) { s.Spec.Target = 0.9 },
		"alerting off":   func(s *ServiceLevelObjective) { s.Spec.Policies = &[]BurnRatePolicy{} },
		"pushed samples": func(s *ServiceLevelObjective) { s.Spec.Queries = nil },
		"two sources": func(s *ServiceLevelObjective) {
			s.Spec.Binding = &BindingReference{Name: "b", Revision: 1, Digest: "d"}
		},
		"five minute step": func(s *ServiceLevelObjective) { five := D(300e9); s.Spec.Step = &five },
		"explicit policy": func(s *ServiceLevelObjective) {
			s.Spec.Policies = &[]BurnRatePolicy{{Name: "page", Severity: "critical", ShortWindow: D(300e9), LongWindow: D(3600e9), BurnRate: 14.4, For: ptr(D(120e9)), MinCoverage: ptr(0.9)}}
		},
		"single unnamed policy": func(s *ServiceLevelObjective) {
			s.Spec.Policies = &[]BurnRatePolicy{{ShortWindow: D(300e9), LongWindow: D(3600e9), BurnRate: 14.4, For: ptr(D(120e9))}}
		},
		"latency measurement": func(s *ServiceLevelObjective) {
			s.Spec.Queries = nil
			s.Spec.Measurement = &MeasurementSpec{Type: "latency", Metric: "http_request_duration_seconds", Threshold: ptr(D(300e6)), ObservationPoint: "Handler."}
		},
	}
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := base("api")
		cases[name](&s)
		hub, notes, err := s.ToHub()
		c := importCase{Name: name, Beta: s, Notes: notes}
		if notes == nil {
			c.Notes = []string{}
		}
		if err != nil {
			c.Error = err.Error()
		} else {
			c.Hub = &hub
		}
		out.Import = append(out.Import, c)
	}
	addTopologyFixtures(t, &out)
	return out
}

func TestPortalConversionFixtures(t *testing.T) {
	want, err := json.MarshalIndent(buildFixtures(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := "../../web/src/lib/v1beta1-fixtures.json"
	if os.Getenv("UPDATE_V1BETA1_FIXTURES") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("web/src/lib/v1beta1-fixtures.json is stale; run UPDATE_V1BETA1_FIXTURES=1 go test ./api/v1beta1")
	}
}
