package model

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestIntentStrictYAMLAndExplicitSemantics(t *testing.T) {
	in := v1.SLOIntent{APIVersion: APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "api", Project: "test"}, Spec: v1.IntentSpec{Description: "Successful API attempts", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"environment": "test"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}
	b, _ := yaml.Marshal(in)
	if _, err := ParseIntent(b); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(b) + "---\n{}", strings.Replace(string(b), "target:", "targte:", 1), string(b) + "spec: {}\n", strings.Replace(string(b), "noDataPolicy: unknown", "noDataPolicy: success", 1), strings.Replace(string(b), "outcome: availability", "outcome: availability-and-latency", 1)} {
		if _, err := ParseIntent([]byte(text)); err == nil {
			t.Fatal("invalid intent accepted", text)
		}
	}
	in.Spec.Population = nil
	if err := ValidateIntent(in); err == nil {
		t.Fatal("implicit population accepted")
	}
}

func TestIntentObservationExclusionsAreExplicitAndBounded(t *testing.T) {
	in := v1.SLOIntent{APIVersion: APIVersion, Kind: v1.IntentKind, Metadata: v1.Metadata{Name: "api", Project: "test"}, Spec: v1.IntentSpec{Description: "Completed attempts", Service: "api", Source: "primary", Outcome: "availability", Population: map[string]string{"service": "api"}, Target: .999, WindowSeconds: 3600, NoDataPolicy: "unknown"}}
	for _, tc := range []struct {
		name  string
		value *v1.IntentExclusions
		valid bool
	}{
		{"not-recorded", nil, true}, {"explicit-none", &v1.IntentExclusions{None: true}, true}, {"boundary", &v1.IntentExclusions{ObservationBoundaries: []string{"Requests never reaching this observer are not measured."}}, true},
		{"empty", &v1.IntentExclusions{}, false}, {"both", &v1.IntentExclusions{None: true, ObservationBoundaries: []string{"x"}}, false}, {"empty-list", &v1.IntentExclusions{None: true, ObservationBoundaries: []string{}}, false}, {"duplicate", &v1.IntentExclusions{ObservationBoundaries: []string{"same", " SAME "}}, false}, {"blank", &v1.IntentExclusions{ObservationBoundaries: []string{" "}}, false}, {"oversize", &v1.IntentExclusions{ObservationBoundaries: []string{strings.Repeat("x", 501)}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in.Spec.Exclusions = tc.value
			if (ValidateIntent(in) == nil) != tc.valid {
				t.Fatal("wrong exclusion validity")
			}
			if tc.valid {
				raw, _ := yaml.Marshal(in)
				parsed, err := ParseIntent(raw)
				if err != nil || (parsed.Spec.Exclusions == nil) != (tc.value == nil) {
					t.Fatal("lost explicit exclusion state", err)
				}
			}
		})
	}
}
