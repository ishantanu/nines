package v1alpha1

import "time"

// CustomerRef names one customer an SLO is also reported for. Value is the
// metric label value that identifies the customer's traffic.
type CustomerRef struct {
	Value  string   `json:"value" yaml:"value"`
	Name   string   `json:"name,omitempty" yaml:"name,omitempty"`
	Target *float64 `json:"target,omitempty" yaml:"target,omitempty"`
}

// CustomerPolicy opts an SLO into per-customer reporting for a small, explicit
// allowlist. It never changes the SLO's own measurement, alerts or budget. The
// label must vary across customers, so it cannot be one the measurement pins.
type CustomerPolicy struct {
	Label     string        `json:"label" yaml:"label"`
	Customers []CustomerRef `json:"customers" yaml:"customers"`
}

// CustomerResult is one customer's reliability over the SLO window.
// States: observed, low_traffic, no_traffic, missing_numerator, no_series.
// A customer with no data is never reported as healthy.
type CustomerResult struct {
	Name                    string   `json:"name"`
	Alias                   string   `json:"alias"`
	State                   string   `json:"state"`
	Good                    *float64 `json:"good"`
	Total                   float64  `json:"total"`
	Target                  float64  `json:"target"`
	Reliability             *float64 `json:"reliability"`
	BurnRate                *float64 `json:"burnRate"`
	BudgetRemainingFraction *float64 `json:"budgetRemainingFraction"`
	// MaskedByAggregate is true when this customer burns budget faster than
	// sustainably (burn above 1) while the service-wide burn does not.
	MaskedByAggregate bool `json:"maskedByAggregate"`
}

// CustomerReport is an explicit, bounded provider read at one cutoff.
type CustomerReport struct {
	APIVersion      string           `json:"apiVersion"`
	Tenant          string           `json:"tenant"`
	SLOID           string           `json:"sloId"`
	Revision        int64            `json:"revision"`
	State           string           `json:"state"`
	Message         string           `json:"message"`
	At              time.Time        `json:"at"`
	WindowSeconds   int64            `json:"windowSeconds"`
	Label           string           `json:"label,omitempty"`
	ServiceBurnRate *float64         `json:"serviceBurnRate"`
	Items           []CustomerResult `json:"items"`
	Limitations     []string         `json:"limitations"`
}
