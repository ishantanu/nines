package v1alpha1

import "time"

// CompositionPromise is a committed availability promise made by a dependency.
type CompositionPromise struct {
	Service string  `json:"service"`
	SLOID   string  `json:"sloId"`
	Target  float64 `json:"target"`
}

// CompositionCheck compares one availability SLO with the ceiling its critical
// dependencies place on it. Verdicts:
//   - exceeds_ceiling: the target is above what the known dependencies allow.
//   - within_ceiling: every critical dependency has a committed availability
//     promise and the target is at or below their product.
//   - within_known_ceiling: the target fits the known dependencies, but at
//     least one critical dependency has no committed availability SLO, so the
//     true ceiling can only be lower.
//   - no_ceiling: no critical dependency has a committed availability SLO.
type CompositionCheck struct {
	Service      string               `json:"service"`
	SLOID        string               `json:"sloId"`
	Target       float64              `json:"target"`
	Verdict      string               `json:"verdict"`
	Ceiling      *float64             `json:"ceiling,omitempty"`
	Headroom     *float64             `json:"headroom,omitempty"`
	Dependencies []CompositionPromise `json:"dependencies"`
	Unmeasured   []string             `json:"unmeasuredDependencies"`
}

// DependencyComposition is the serial-composition check for one dependency map.
// It is a deterministic upper bound from declared critical edges only.
type DependencyComposition struct {
	APIVersion  string             `json:"apiVersion"`
	Tenant      string             `json:"tenant"`
	ID          string             `json:"id"`
	Revision    int64              `json:"revision"`
	ObservedAt  time.Time          `json:"observedAt"`
	Checks      []CompositionCheck `json:"checks"`
	Limitations []string           `json:"limitations"`
}
