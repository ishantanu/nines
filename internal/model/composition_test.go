package model

import (
	"math"
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestComposeAvailabilityIsAnUpperBoundFromCriticalEdges(t *testing.T) {
	deps := []v1.DeclaredDependency{
		{Caller: "web", Callee: "api", Critical: true},
		{Caller: "web", Callee: "db", Critical: true},
		{Caller: "web", Callee: "db", Critical: true}, // duplicate edge counts once
		{Caller: "web", Callee: "cache", Critical: false},
		{Caller: "web", Callee: "web", Critical: true}, // self edge ignored
	}
	facts := map[string][]AvailabilityFact{
		"web": {{SLOID: "web-b", Target: 0.99}, {SLOID: "web-a", Target: 0.9995}},
		"api": {{SLOID: "api-slo", Target: 0.999}},
		"db":  {{SLOID: "db-strict", Target: 0.9999}, {SLOID: "db-loose", Target: 0.999}},
	}
	got := ComposeAvailability(deps, facts)
	byID := map[string]v1.CompositionCheck{}
	for _, c := range got {
		byID[c.SLOID] = c
	}
	// Weakest db promise 0.999 and api 0.999 compose to 0.998001.
	want := 0.999 * 0.999
	a := byID["web-a"]
	if a.Ceiling == nil || math.Abs(*a.Ceiling-want) > 1e-12 || a.Verdict != "exceeds_ceiling" {
		t.Fatalf("web-a = %+v", a)
	}
	if a.Headroom == nil || *a.Headroom >= 0 {
		t.Fatalf("headroom must be negative when the target exceeds the ceiling: %+v", a.Headroom)
	}
	if len(a.Dependencies) != 2 || a.Dependencies[1].SLOID != "db-loose" {
		t.Fatalf("the weakest promise per dependency must be used: %+v", a.Dependencies)
	}
	if byID["web-b"].Verdict != "within_ceiling" {
		t.Fatalf("0.99 fits under 0.998001: %+v", byID["web-b"])
	}
	// The dependency services have no critical callees, so they have no ceiling.
	if byID["api-slo"].Verdict != "no_ceiling" || byID["api-slo"].Ceiling != nil {
		t.Fatalf("api = %+v", byID["api-slo"])
	}
	if got[0].SLOID != "api-slo" || got[len(got)-1].SLOID != "web-b" {
		t.Fatalf("output must be sorted by service then SLO: %+v", got)
	}
}

func TestComposeAvailabilityNeverClaimsCompletenessWithUnmeasuredDependencies(t *testing.T) {
	deps := []v1.DeclaredDependency{{Caller: "web", Callee: "api", Critical: true}, {Caller: "web", Callee: "queue", Critical: true}}
	facts := map[string][]AvailabilityFact{"web": {{SLOID: "web", Target: 0.99}}, "api": {{SLOID: "api", Target: 0.999}}}
	c := ComposeAvailability(deps, facts)[1]
	if c.SLOID != "web" || c.Verdict != "within_known_ceiling" || len(c.Unmeasured) != 1 || c.Unmeasured[0] != "queue" {
		t.Fatalf("partial knowledge must not read as a confirmed ceiling: %+v", c)
	}
	facts["web"][0].Target = 0.9995
	if c = ComposeAvailability(deps, facts)[1]; c.Verdict != "exceeds_ceiling" {
		t.Fatalf("an excess is certain even with partial knowledge: %+v", c)
	}
	none := ComposeAvailability(deps, map[string][]AvailabilityFact{"web": {{SLOID: "web", Target: 0.99}}})
	if none[0].Verdict != "no_ceiling" || none[0].Ceiling != nil {
		t.Fatalf("no committed dependency promise means no ceiling: %+v", none[0])
	}
	exact := ComposeAvailability([]v1.DeclaredDependency{{Caller: "a", Callee: "b", Critical: true}}, map[string][]AvailabilityFact{"a": {{SLOID: "a", Target: 0.999}}, "b": {{SLOID: "b", Target: 0.999}}})
	if exact[0].Verdict != "within_ceiling" || exact[0].Headroom == nil || *exact[0].Headroom != 0 {
		t.Fatalf("a target equal to the ceiling is allowed: %+v", exact[0])
	}
}
