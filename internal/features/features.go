// Package features decides which experimental capabilities a server offers.
//
// Stable and beta capabilities are always on. Gated experimental capabilities
// are off unless the server turns them on, so a default installation offers
// only the stable core. The gate names are the feature IDs in the maturity
// catalog, so one list names every gate.
package features

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ishantanu/nines/internal/maturity"
)

// Env names the environment variable that turns on gated features.
const Env = "NINES_FEATURES"

// Set is the server's feature configuration. The zero value turns on no
// gated feature.
type Set struct {
	all bool
	on  map[string]bool
}

// All turns on every gated feature, as the synthetic demo does.
func All() Set { return Set{all: true} }

// Parse reads a comma-separated list of gated feature IDs, "all" or "none".
// Unknown IDs are rejected so a typo cannot silently leave a feature off.
func Parse(spec string) (Set, error) {
	s := Set{on: map[string]bool{}}
	for _, raw := range strings.Split(spec, ",") {
		id := strings.TrimSpace(raw)
		switch id {
		case "", "none":
			continue
		case "all":
			s.all = true
			continue
		}
		f, ok := maturity.Lookup(id)
		if !ok || !f.Gated {
			return Set{}, fmt.Errorf("%s: %q is not a gated feature; use all, none or any of: %s", Env, id, strings.Join(Gated(), ", "))
		}
		s.on[id] = true
	}
	return s, nil
}

// Gated lists every gated feature ID.
func Gated() []string {
	out := []string{}
	for _, f := range maturity.Catalog() {
		if f.Gated {
			out = append(out, f.ID)
		}
	}
	sort.Strings(out)
	return out
}

// Enabled reports whether the server offers a capability. Anything that is not
// a gated feature is always on.
func (s Set) Enabled(id string) bool {
	f, ok := maturity.Lookup(id)
	if !ok || !f.Gated {
		return true
	}
	return s.all || s.on[id]
}

// List returns the gated features this server turns on, for clients.
func (s Set) List() []string {
	out := []string{}
	for _, id := range Gated() {
		if s.Enabled(id) {
			out = append(out, id)
		}
	}
	return out
}

// ProviderEnabled reports whether a telemetry provider kind may be used.
func (s Set) ProviderEnabled(kind string) bool {
	for _, f := range maturity.Catalog() {
		if f.Provider == kind {
			return s.Enabled(f.ID)
		}
	}
	return true
}

// routes maps API path prefixes to the gated feature that owns them.
var routes = []struct{ prefix, id string }{
	{"/api/intents", "create-from-goal"},
	{"/api/intent-conversations", "create-from-goal"},
	{"/api/intent-context", "create-from-goal"},
	{"/api/investigations", "reliability-reviews"},
	{"/api/incidents", "incidents"},
	{"/api/review-skills", "reliability-reviews"},
	{"/api/action-items", "reliability-reviews"},
	{"/api/workflows", "workflows"},
	{"/api/visual-evaluations", "agent-evidence"},
	{"/api/session-replays", "session-replay"},
	{"/api/migrations", "migration"},
	{"/api/dependency-discovery", "observed-dependencies"},
}

// ForPath returns the gated feature that owns an API path, if any.
func ForPath(path string) (string, bool) {
	for _, r := range routes {
		if path == r.prefix || strings.HasPrefix(path, r.prefix+"/") {
			return r.id, true
		}
	}
	// Journey-scoped agent evidence: /api/journeys/{id}/visual-evaluations...
	if strings.HasPrefix(path, "/api/journeys/") && strings.Contains(path, "/visual-evaluations") {
		return "agent-evidence", true
	}
	return "", false
}

// Disabled explains a refused request and how an administrator turns the
// feature on.
func Disabled(id string) string {
	name := id
	if f, ok := maturity.Lookup(id); ok {
		name = f.Name
	}
	return fmt.Sprintf("%s is an experimental feature that is turned off on this server. An administrator can turn it on with %s=%s. See https://ishantanu.github.io/nines/docs/maturity/#experimental", name, Env, id)
}
