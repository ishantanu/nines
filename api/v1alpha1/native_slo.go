package v1alpha1

import "time"

// NativeSLOResults is independent provider evidence, never Nines measurement input.
// Ratios use 0..1; remaining budget can be negative. Null means unavailable.
type NativeSLOResults struct {
	APIVersion      string           `json:"apiVersion"`
	Kind            string           `json:"kind"`
	Tenant          string           `json:"tenant"`
	SLOID           string           `json:"sloId"`
	Revision        int64            `json:"revision"`
	Provider        string           `json:"provider"`
	Source          string           `json:"source"`
	NativeID        string           `json:"nativeID"`
	URL             string           `json:"url"`
	State           string           `json:"state"`
	From            time.Time        `json:"from"`
	To              time.Time        `json:"to"`
	CheckedAt       time.Time        `json:"checkedAt"`
	Target          float64          `json:"target"`
	Attainment      *float64         `json:"attainment"`
	BudgetRemaining *float64         `json:"budgetRemaining"`
	BudgetBasis     string           `json:"budgetBasis"`
	Points          []NativeSLOPoint `json:"points"`
	Warnings        []string         `json:"warnings"`
}
type NativeSLOPoint struct {
	Time       time.Time `json:"time"`
	Attainment *float64  `json:"attainment"`
}

// NativeSLOCatalog is bounded and read-only. It never adopts provider ownership.
type NativeSLOCatalog struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Tenant     string             `json:"tenant"`
	Source     string             `json:"source"`
	Provider   string             `json:"provider"`
	Items      []NativeSLOSummary `json:"items"`
}
type NativeSLOSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Importable bool   `json:"importable"`
	Reason     string `json:"reason,omitempty"`
}
type NativeSLODraft struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Tenant     string   `json:"tenant"`
	Source     string   `json:"source"`
	NativeID   string   `json:"nativeID"`
	Manifest   string   `json:"manifest"`
	Warnings   []string `json:"warnings"`
}
