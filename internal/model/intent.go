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
	if err := validateBudgetPolicy(s.ErrorBudgetPolicy); err != nil {
		return err
	}
	if err := validateQuality(s.Quality, s.StepSeconds); err != nil {
		return err
	}
	if !s.Lifecycle.Valid() {
		return Invalid("invalid intent lifecycle")
	}
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
	if s.StepSeconds < 0 || s.StepSeconds > 3600 {
		return Invalid("intent stepSeconds must be 1–3600 when supplied")
	}
	if err := validateBurnPolicies(SLO{Target: s.Target, WindowSeconds: s.WindowSeconds, StepSeconds: s.StepSeconds, Policies: s.Policies, Policy: s.Policy}); err != nil {
		return err
	}
	if s.NoDataPolicy != "unknown" {
		return Invalid("noDataPolicy must explicitly be unknown; missing data cannot establish success")
	}
	if j := s.Journey; j != nil {
		if !identifier.MatchString(j.Name) || !identifier.MatchString(j.Step) || j.ExpectedRevision < 1 {
			return Invalid("journey needs a name, step and positive expectedRevision")
		}
	}
	if s.Measurement != nil {
		probe := SLO{ID: "intent", Project: "intent", Name: "Intent", Service: s.Service, Source: s.Source, Target: s.Target, WindowSeconds: s.WindowSeconds, StepSeconds: s.StepSeconds, Measurement: s.Measurement, ProviderWindow: MeasurementWindow(*s.Measurement)}
		if err := ValidateInlineSLO(probe); err != nil {
			return err
		}
		if !IntentMatchesBinding(v1.SLOIntent{Metadata: v1.Metadata{Project: "intent"}, Spec: s}, InlineBinding(probe)) {
			return Invalid("inline measurement must match the intent outcome, entire population and latency threshold")
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
	if DefinitionDigest(i.ErrorBudgetPolicy) != DefinitionDigest(s.ErrorBudgetPolicy) {
		return Invalid("SLO error-budget policy must agree with its reviewed intent")
	}
	if DefinitionDigest(i.Quality) != DefinitionDigest(s.Quality) {
		return Invalid("SLO quality policy must agree with its reviewed intent")
	}
	if i.Lifecycle.Effective() != s.Lifecycle.Effective() {
		return Invalid("SLO lifecycle must agree with its reviewed intent")
	}
	if s.Measurement != nil && (i.StepSeconds != s.StepSeconds || DefinitionDigest(i.Measurement) != DefinitionDigest(s.Measurement)) {
		return Invalid("SLO measurement and stepSeconds must agree with its reviewed intent")
	}
	if s.Measurement == nil && i.Measurement != nil {
		return Invalid("inline intent must retain its measurement")
	}
	if (s.MeasurementBinding == nil && s.Measurement == nil) || s.ProviderWindow == nil || i.Service != s.Service || i.Source != s.Source || i.Target != s.Target || i.WindowSeconds != s.WindowSeconds || (DefinitionDigest(i.Policy) != DefinitionDigest(s.Policy) || DefinitionDigest(i.Policies) != DefinitionDigest(s.Policies)) {
		return Invalid("compiled SLO must agree with its intent and retain its reviewed measurement")
	}
	return nil
}
