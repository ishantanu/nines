package model

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
