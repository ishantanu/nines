package features

import (
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/maturity"
)

func TestDefaultOffersOnlyTheStableCore(t *testing.T) {
	var none Set
	for _, f := range maturity.Catalog() {
		if got, want := none.Enabled(f.ID), !f.Gated; got != want {
			t.Fatalf("%s enabled by default = %v, want %v", f.ID, got, want)
		}
	}
	if len(none.List()) != 0 {
		t.Fatal("the zero value turned a gated feature on")
	}
	if !none.ProviderEnabled("prometheus") || none.ProviderEnabled("datadog") || !none.ProviderEnabled("manual") {
		t.Fatal("provider gates do not follow the catalog")
	}
}

func TestParse(t *testing.T) {
	s, err := Parse(" session-replay , datadog,none,")
	if err != nil || !s.Enabled("session-replay") || !s.ProviderEnabled("datadog") || s.Enabled("workflows") {
		t.Fatalf("list parsed wrongly: %v %v", s.List(), err)
	}
	if all, _ := Parse("all"); len(all.List()) != len(Gated()) {
		t.Fatal("all does not turn every gated feature on")
	}
	for _, bad := range []string{"session_replay", "slo-definitions", "git-delivery"} {
		if _, err := Parse(bad); err == nil || !strings.Contains(err.Error(), "not a gated feature") {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
}

// Every gated feature with a portal page or API has a route, so turning it
// off cannot leave its API reachable.
func TestEveryGatedFeatureOwnsItsRoutes(t *testing.T) {
	owned := map[string]bool{}
	for _, r := range routes {
		owned[r.id] = true
	}
	for _, f := range maturity.Catalog() {
		if f.Gated && f.Provider == "" && !owned[f.ID] {
			t.Fatalf("gated feature %s has no API route", f.ID)
		}
	}
	cases := map[string]string{
		"/api/intents/plan":                         "create-from-goal",
		"/api/intent-conversations/x":               "create-from-goal",
		"/api/investigations":                       "reliability-reviews",
		"/api/session-replays/abc/links":            "session-replay",
		"/api/journeys/checkout/visual-evaluations": "agent-evidence",
		"/api/dependency-discovery/kubernetes":      "observed-dependencies",
		"/api/migrations/convert":                   "migration",
		"/api/workflows":                            "workflows",
	}
	for path, want := range cases {
		if got, ok := ForPath(path); !ok || got != want {
			t.Fatalf("%s owned by %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{"/api/slos", "/api/journeys/checkout", "/api/intentsx", "/api/workspace", "/api/dependency-maps"} {
		if id, ok := ForPath(path); ok {
			t.Fatalf("%s wrongly gated by %s", path, id)
		}
	}
}

func TestDisabledMessageSaysHowToTurnItOn(t *testing.T) {
	msg := Disabled("session-replay")
	if !strings.Contains(msg, "Session replay") || !strings.Contains(msg, "NINES_FEATURES=session-replay") {
		t.Fatal(msg)
	}
}
