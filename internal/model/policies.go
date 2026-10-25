package model

import v1 "github.com/ishantanu/nines/api/v1alpha1"

func (s SLO) AlertsEnabled() bool { return s.Lifecycle.Effective() == v1.LifecycleCommitted }

// AlertingPolicies controls alert emission only. All configured windows remain
// available for measurement and validation regardless of lifecycle.
func (s SLO) AlertingPolicies() []Policy {
	if !s.AlertsEnabled() {
		return nil
	}
	return s.AlertPolicies()
}

// LifecycleAlertState also describes suppression before the first evaluation.
func (s SLO) LifecycleAlertState(state AlertState) AlertState {
	if s.AlertsEnabled() {
		return state
	}
	return AlertState{Phase: "inactive", SuppressedReason: "lifecycle_" + string(s.Lifecycle.Effective())}
}

// AlertPolicies preserves the single-policy input while exposing one compiler
// and evaluator path for named severity policies.
func (s SLO) AlertPolicies() []Policy {
	if len(s.Policies) > 0 {
		return s.Policies
	}
	if s.Policy != nil {
		return []Policy{*s.Policy}
	}
	return nil
}

// A zero step is permitted only while a legacy intent awaits a selected
// measurement. Window bounds and semantic policy checks still apply offline.
func validateBurnPolicies(s SLO) error {
	if len(s.Policies) > 8 || (s.Policy != nil && len(s.Policies) > 0) {
		return Invalid("use either policy or up to eight named policies")
	}
	seenPolicies := map[string]bool{}
	for _, p := range s.AlertPolicies() {
		if len(s.Policies) > 0 && (!identifier.MatchString(p.Name) || seenPolicies[p.Name] || (p.Severity != "critical" && p.Severity != "warning" && p.Severity != "info")) {
			return Invalid("policies require unique names and critical, warning or info severity")
		}
		seenPolicies[p.Name] = true
		if !Finite(p.MinEvents) || p.MinEvents < 0 || p.MinEvents > 1e18 {
			return Invalid("minEvents must be a finite count between 0 and 1e18")
		}
		maximum := 1 / (1 - s.Target)
		if p.BurnRate > maximum*(1+1e-12) {
			return Invalid("burnRate %g is unreachable for target %g; maximum is %g", p.BurnRate, s.Target, maximum)
		}
		if p.ShortWindowSeconds < max(s.StepSeconds, 1) || p.LongWindowSeconds < p.ShortWindowSeconds || p.LongWindowSeconds > s.WindowSeconds || (s.StepSeconds > 0 && (p.ShortWindowSeconds%s.StepSeconds != 0 || p.LongWindowSeconds%s.StepSeconds != 0)) {
			return Invalid("alert windows must be aligned multiples of stepSeconds within the SLO window")
		}
		if !Finite(p.BurnRate) || p.BurnRate <= 0 || !Finite(p.MinCoverage) || p.MinCoverage <= 0 || p.MinCoverage > 1 || p.ForSeconds < 0 || p.ForSeconds > 31*86400 {
			return Invalid("invalid burn-rate, coverage or pending duration")
		}
	}
	return nil
}
