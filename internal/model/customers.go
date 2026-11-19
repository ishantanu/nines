package model

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Per-customer reporting is opt-in and bounded. The allowlist is explicit, so
// nothing about it scales with the number of customers a service has.
const MaxCustomers = 20

var (
	customerValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,127}$`)
	customerName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/-]{0,62}$`)
)

func validateCustomers(s SLO) error {
	c := s.Customers
	if c == nil {
		return nil
	}
	if s.Measurement != nil && s.Measurement.Type == "sessions" {
		return Invalid("customers does not support session measurements, which derive good from two counters")
	}
	if s.ProviderWindow == nil || s.Measurement == nil {
		return Invalid("customers requires an inline measurement, whose provider windows can be grouped by a label")
	}
	if !LabelName.MatchString(c.Label) || strings.HasPrefix(c.Label, "__") || c.Label == "le" {
		return Invalid("customers.label must be a metric label name other than le")
	}
	if _, pinned := s.Measurement.Labels[c.Label]; pinned {
		return Invalid("customers.label %q is fixed by the measurement population, so it cannot separate customers", c.Label)
	}
	if _, pinned := s.Measurement.SuccessLabels[c.Label]; pinned {
		return Invalid("customers.label %q is used to classify success, so it cannot identify a customer", c.Label)
	}
	if len(c.Customers) == 0 || len(c.Customers) > MaxCustomers {
		return Invalid("customers must list 1 to %d customers", MaxCustomers)
	}
	seen := map[string]bool{}
	for _, customer := range c.Customers {
		if !customerValue.MatchString(customer.Value) {
			return Invalid("a customer value must be 1 to 128 letters, digits, dots, colons, slashes, at signs, hyphens or underscores, starting with a letter or digit")
		}
		if seen[customer.Value] {
			return Invalid("customer %q is listed twice", customer.Value)
		}
		seen[customer.Value] = true
		if customer.Name != "" && !customerName.MatchString(customer.Name) {
			return Invalid("a customer name must be 1 to 63 letters, digits, spaces, dots, slashes, hyphens or underscores")
		}
		if customer.Target != nil && (!Finite(*customer.Target) || *customer.Target <= 0 || *customer.Target >= 1) {
			return Invalid("a customer target must be above 0 and below 1")
		}
	}
	return nil
}

// CustomerAlias is a stable, non-reversible handle for a customer within one
// SLO. Logs, audit entries and metrics use it instead of the raw identifier.
func CustomerAlias(tenant, sloID, value string) string {
	sum := sha256.Sum256([]byte(tenant + "/" + sloID + "/" + value))
	return "customer-" + hex.EncodeToString(sum[:4])
}
