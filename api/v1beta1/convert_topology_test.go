package v1beta1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

// shipped returns every shipped example of one kind.
func shipped(t *testing.T, kind string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	files, _ := filepath.Glob("testdata/v1alpha1/*.yaml")
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "\nkind: "+kind+"\n") {
			out[filepath.Base(f)] = raw
		}
	}
	return out
}

func TestShippedBindingsRoundTrip(t *testing.T) {
	examples := shipped(t, BindingKind)
	if len(examples) == 0 {
		t.Fatal("no binding examples")
	}
	for name, raw := range examples {
		var hub v1alpha1.MeasurementBinding
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(name, err)
		}
		text, _ := yaml.Marshal(BindingFromHub(hub))
		if strings.Contains(string(text), "Seconds") || strings.Contains(string(text), "measurement:") {
			t.Fatalf("%s: v1alpha1 field names in v1beta1 output:\n%s", name, text)
		}
		var reread MeasurementBinding
		if err := yaml.Unmarshal(text, &reread); err != nil {
			t.Fatal(name, err)
		}
		back, err := reread.ToHub()
		if err != nil {
			t.Fatal(name, err)
		}
		same(t, name, hub, back)
	}
}

func TestShippedDependencyMapsRoundTrip(t *testing.T) {
	hub := v1alpha1.ServiceDependencyMap{APIVersion: v1alpha1.APIVersion, Kind: DependencyMapKind, Metadata: Metadata{Name: "commerce", Project: "p"}, Spec: v1alpha1.DependencyMapSpec{DisplayName: "Commerce", Services: []v1alpha1.MapService{{Name: "api", DisplayName: "API", SLORefs: []string{"api"}}}, Dependencies: []v1alpha1.DeclaredDependency{{Caller: "api", Callee: "db", Critical: true}}, Observed: &v1alpha1.GraphObservation{Source: "primary", Labels: map[string]string{"cluster": "a"}, WindowSeconds: 3600, ScrapeIntervalSeconds: 30}}}
	examples := shipped(t, DependencyMapKind)
	examples["inline"] = nil
	for name, raw := range examples {
		h := hub
		if raw != nil {
			h = v1alpha1.ServiceDependencyMap{}
			if err := yaml.Unmarshal(raw, &h); err != nil {
				t.Fatal(name, err)
			}
		}
		text, _ := yaml.Marshal(DependencyMapFromHub(h))
		var reread ServiceDependencyMap
		if err := yaml.Unmarshal(text, &reread); err != nil {
			t.Fatal(name, err)
		}
		back, err := reread.ToHub()
		if err != nil {
			t.Fatal(name, err)
		}
		same(t, name, h, back)
	}
}
