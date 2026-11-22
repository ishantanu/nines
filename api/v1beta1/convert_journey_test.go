package v1beta1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1alpha1 "github.com/ishantanu/nines/api/v1alpha1"
)

func lookup(specs map[string]v1alpha1.SLOSpec) SLOLookup {
	return func(id string) (v1alpha1.SLOSpec, bool) { s, ok := specs[id]; return s, ok }
}

var latencySLO = v1alpha1.SLOSpec{Measurement: &v1alpha1.MeasurementSpec{Type: "latency"}}

func journey(links []JourneySLO, refs ...string) CriticalUserJourney {
	return CriticalUserJourney{APIVersion: APIVersion, Kind: JourneyKind, Metadata: Metadata{Name: "checkout", Project: "p"}, Spec: JourneySpec{DisplayName: "Checkout", Owner: "team", Criticality: "high", SLOs: links, Steps: []JourneyStep{{ID: "pay", Name: "Pay", Actor: "user", SLORefs: refs}}}}
}

func TestJourneyDimensionFollowsTheSLO(t *testing.T) {
	slos := lookup(map[string]v1alpha1.SLOSpec{"api": {}, "search": latencySLO})
	hub, _, err := journey([]JourneySLO{{ID: "api", Required: true}, {ID: "search"}, {ID: "spend", Dimension: "cost"}}, "api").ToHub(slos)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, l := range hub.Spec.SLOs {
		got = append(got, l.Dimension)
	}
	if strings.Join(got, ",") != "availability,latency,cost" {
		t.Fatal(got)
	}
	if _, _, err := journey([]JourneySLO{{ID: "search", Dimension: "availability"}}, "search").ToHub(slos); err == nil || !strings.Contains(err.Error(), "contradicts the SLO, which measures latency") {
		t.Fatal(err)
	}
	if _, notes, err := journey([]JourneySLO{{ID: "unknown"}}, "unknown").ToHub(nil); err != nil || !strings.Contains(strings.Join(notes, " "), "assumed to be availability") {
		t.Fatal(notes, err)
	}
	if _, _, err := journey([]JourneySLO{{ID: "api"}}, "other").ToHub(slos); err == nil || !strings.Contains(err.Error(), "not in slos") {
		t.Fatal(err)
	}
}

func TestShippedJourneysRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("../../examples/*.journey.yaml")
	if len(files) < 2 {
		t.Fatal("examples not found")
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		var hub v1alpha1.CriticalUserJourney
		if err := yaml.Unmarshal(raw, &hub); err != nil {
			t.Fatal(f, err)
		}
		// Every linked SLO is a plain availability SLO in this fixture, so
		// matching dimensions disappear from the export and come back.
		specs := map[string]v1alpha1.SLOSpec{}
		for _, l := range hub.Spec.SLOs {
			specs[l.ID] = v1alpha1.SLOSpec{}
			switch l.Dimension {
			case "latency":
				specs[l.ID] = latencySLO
			case "correctness":
				specs[l.ID] = v1alpha1.SLOSpec{Measurement: &v1alpha1.MeasurementSpec{Type: "correctness"}}
			}
		}
		beta := JourneyFromHub(hub, lookup(specs))
		text, _ := yaml.Marshal(beta)
		var reread CriticalUserJourney
		if err := yaml.Unmarshal(text, &reread); err != nil {
			t.Fatal(f, err)
		}
		back, _, err := reread.ToHub(lookup(specs))
		if err != nil {
			t.Fatal(f, err)
		}
		same(t, filepath.Base(f), hub, back)
	}
}
