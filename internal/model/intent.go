package model

import v1 "github.com/ishantanu/nines/api/v1alpha1"

func ParseIntent(body []byte) (v1.SLOIntent, error) {
	var in v1.SLOIntent
	if err := decodeDefinition(body, &in); err != nil {
		return in, err
	}
	return in, ValidateIntent(in)
}
func ValidateIntent(in v1.SLOIntent) error {
	if in.APIVersion != APIVersion || in.Kind != v1.IntentKind {
		return Invalid("expected apiVersion %s and kind %s", APIVersion, v1.IntentKind)
	}
	if !identifier.MatchString(in.Metadata.Name) || !validText(in.Metadata.Project, 200) {
		return Invalid("intent needs a valid name and project")
	}
	return ValidateIntentSpec(in.Spec)
}
func ValidateIntentSpec(s v1.IntentSpec) error {
	if !validText(s.Description, 2000) || !validText(s.Service, 200) || !identifier.MatchString(s.Source) {
		return Invalid("description, service and configured source are required")
	}
	if !oneOf(s.Outcome, "availability", "latency", "freshness", "correctness") {
		return Invalid("outcome must be availability, latency, freshness or correctness; combined outcomes need separate definitions")
	}
	if len(s.Population) == 0 {
		return Invalid("an explicit population is required; environment and eligible traffic are never inferred")
	}
	if err := ValidateLabels(s.Population); err != nil {
		return err
	}
	if s.Outcome == "latency" {
		if s.ThresholdSeconds == nil || !Finite(*s.ThresholdSeconds) || *s.ThresholdSeconds <= 0 || *s.ThresholdSeconds > 86400 {
			return Invalid("latency requires a positive thresholdSeconds at most 86400")
		}
	} else if s.ThresholdSeconds != nil {
		return Invalid("thresholdSeconds is supported only for latency")
	}
	if !Finite(s.Target) || s.Target <= 0 || s.Target >= 1 || s.WindowSeconds < 1 || s.WindowSeconds > 31*86400 {
		return Invalid("target must be between 0 and 1 exclusively and windowSeconds within 31 days")
	}
	if s.NoDataPolicy != "unknown" {
		return Invalid("noDataPolicy must explicitly be unknown; missing data cannot establish success")
	}
	if j := s.Journey; j != nil {
		if !identifier.MatchString(j.Name) || !identifier.MatchString(j.Step) || j.ExpectedRevision < 1 {
			return Invalid("journey needs a name, step and positive expectedRevision")
		}
	}
	return nil
}

// IntentMatchesBinding requires the entire eligible population, not a subset or
// fuzzy name match. A human still reviews success semantics and observation point.
func IntentMatchesBinding(i v1.SLOIntent, b MeasurementBinding) bool {
	s, t := i.Spec, b.Spec
	if i.Metadata.Project != b.Metadata.Project || s.Service != t.Service || s.Source != t.Source || s.Outcome != t.Measurement || len(s.Population) != len(t.Labels) {
		return false
	}
	for k, v := range s.Population {
		if got, ok := t.Labels[k]; !ok || got != v {
			return false
		}
	}
	if s.Outcome == "latency" {
		return s.ThresholdSeconds != nil && t.ThresholdSeconds != nil && *s.ThresholdSeconds == *t.ThresholdSeconds
	}
	return s.Outcome == "availability"
}
func validateSLOIntent(s SLO) error {
	if s.Intent == nil {
		return nil
	}
	if err := ValidateIntentSpec(*s.Intent); err != nil {
		return err
	}
	i := s.Intent
	if s.MeasurementBinding == nil || s.ProviderWindow == nil || i.Service != s.Service || i.Source != s.Source || i.Target != s.Target || i.WindowSeconds != s.WindowSeconds || (DefinitionDigest(i.Policy) != DefinitionDigest(s.Policy) || DefinitionDigest(i.Policies) != DefinitionDigest(s.Policies)) {
		return Invalid("compiled SLO must agree with its intent and retain a provider-window binding")
	}
	return nil
}
