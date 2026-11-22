package v1beta1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

// normalize maps a stored SLO to the form v1beta1 can express: the legacy
// single policy becomes a list, provenance and derived window selectors are
// server-owned, and "primary" is the default connection.
func normalize(h v1alpha1.ServiceLevelObjective) v1alpha1.ServiceLevelObjective {
	s := &h.Spec
	s.Intent = nil
	if s.Measurement != nil || s.MeasurementBinding != nil {
		s.ProviderWindow = nil
	}
	return h
}

func same(t *testing.T, name string, a, b any) {
	t.Helper()
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("%s changed in conversion:\n got %s\nwant %s", name, jb, ja)
	}
}

func TestShippedExamplesRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*.slo.yaml")
	if len(files) < 3 {
		t.Fatal("examples not found")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var hub v1alpha1.ServiceLevelObjective
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(f, err)
		}
		beta := FromHub(hub)
		text, err := yaml.Marshal(beta)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(text), "Seconds") {
			t.Fatalf("%s: v1beta1 output still has a seconds field:\n%s", f, text)
		}
		var reread ServiceLevelObjective
		if err := yaml.Unmarshal(text, &reread); err != nil {
			t.Fatal(f, err)
		}
		back, _, err := reread.ToHub()
		if err != nil {
			t.Fatal(f, err)
		}
		same(t, filepath.Base(f), normalize(hub), back)
	}
}

func minimal() ServiceLevelObjective {
	return ServiceLevelObjective{APIVersion: APIVersion, Kind: SLOKind, Metadata: Metadata{Name: "api", Project: "p"}, Spec: SLOSpec{DisplayName: "API", Service: "api", Target: 0.999, Queries: &Queries{Good: "sum(good)", Total: "sum(total)"}}}
}

func TestDefaults(t *testing.T) {
	hub, notes, err := minimal().ToHub()
	if err != nil {
		t.Fatal(err)
	}
	s := hub.Spec
	if s.WindowSeconds != 28*86400 || s.StepSeconds != 60 {
		t.Fatalf("window %d step %d", s.WindowSeconds, s.StepSeconds)
	}
	var names []string
	for _, p := range s.Policies {
		names = append(names, p.Name)
	}
	if !reflect.DeepEqual(names, []string{"fast-burn", "sustained-burn", "slow-burn"}) {
		t.Fatal(names)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "window defaults to 28d") {
		t.Fatal(notes)
	}

	short := minimal()
	day := D(86400e9)
	short.Spec.Window = &day
	hub, notes, _ = short.ToHub()
	if len(hub.Spec.Policies) != 2 || !strings.Contains(strings.Join(notes, "\n"), "slow-burn alert: its 3d window is longer than the 1d SLO window") {
		t.Fatal(hub.Spec.Policies, notes)
	}

	loose := minimal()
	loose.Spec.Target = 0.9
	hub, notes, _ = loose.ToHub()
	if len(hub.Spec.Policies) != 2 || !strings.Contains(strings.Join(notes, "\n"), "14.4x burn rate is unreachable with a 0.9 target") {
		t.Fatal(hub.Spec.Policies, notes)
	}

	silent := minimal()
	silent.Spec.Policies = &[]BurnRatePolicy{}
	hub, notes, _ = silent.ToHub()
	if len(hub.Spec.Policies) != 0 || !strings.Contains(strings.Join(notes, "\n"), "alerting is off") {
		t.Fatal(hub.Spec.Policies, notes)
	}
}

func TestAtMostOneMeasurement(t *testing.T) {
	// No source is an SLO measured from samples pushed to the API.
	pushed := minimal()
	pushed.Spec.Queries = nil
	hub, notes, err := pushed.ToHub()
	if err != nil || hub.Spec.GoodQuery != "" || !strings.Contains(strings.Join(notes, " "), "samples pushed to the API") {
		t.Fatal(notes, err)
	}
	if again := FromHub(hub); again.Spec.Queries != nil || again.Spec.Measurement != nil || again.Spec.Binding != nil {
		t.Fatal("an SLO measured from pushed samples exported a measurement source")
	}
	two := minimal()
	two.Spec.Binding = &BindingReference{Name: "b", Revision: 1, Digest: "d"}
	if _, _, err := two.ToHub(); err == nil || !strings.Contains(err.Error(), "at most one of measurement, queries or binding") {
		t.Fatal("two measurement sources accepted", err)
	}
	both := minimal()
	both.Spec.Intervals = &Intervals{ErrorRate: 0.05, MinGoodEvents: 10}
	if _, _, err := both.ToHub(); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatal(err)
	}
}

// A v1alpha1 SLO without alert policies stays silent after conversion, so an
// upgrade never starts paging anyone.
func TestConvertedSLOWithoutPoliciesStaysSilent(t *testing.T) {
	hub := v1alpha1.NewSLO("quiet", "p", v1alpha1.SLOSpec{DisplayName: "Quiet", Service: "s", Target: 0.99, WindowSeconds: 86400, StepSeconds: 60, GoodQuery: "g", TotalQuery: "t"})
	beta := FromHub(hub)
	if beta.Spec.Policies == nil || len(*beta.Spec.Policies) != 0 {
		t.Fatal("conversion did not keep alerting off")
	}
	text, _ := yaml.Marshal(beta)
	if !strings.Contains(string(text), "policies: []") {
		t.Fatalf("explicit empty policies missing from YAML:\n%s", text)
	}
	back, _, err := beta.ToHub()
	if err != nil || len(back.Spec.Policies) != 0 {
		t.Fatal(back.Spec.Policies, err)
	}
}
