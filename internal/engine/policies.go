package engine

import (
	"time"

	"github.com/ishantanu/nines/internal/model"
)

func AdvanceAllRecord(r model.Record, at time.Time) (model.AlertState, []model.AlertEvent) {
	if len(r.SLO.Policies) == 0 {
		state, event := AdvanceRecord(r, at)
		if event != nil {
			return state, []model.AlertEvent{*event}
		}
		return state, nil
	}
	result := model.AlertState{Phase: "inactive", Policies: map[string]model.AlertState{}}
	result = r.SLO.LifecycleAlertState(result)
	if result.Policies == nil {
		result.Policies = map[string]model.AlertState{}
	}
	var events []model.AlertEvent
	for _, policy := range r.SLO.Policies {
		one := r
		one.SLO.Policies = nil
		one.SLO.Policy = &policy
		one.Alert = r.Alert.Policies[policy.Name]
		digest := model.DefinitionDigest(policy)
		if one.Alert.PolicyDigest != "" && one.Alert.PolicyDigest != digest {
			one.Alert = model.AlertState{Phase: "inactive"}
		}
		state, event := AdvanceRecord(one, at)
		state.Policy, state.Severity, state.PolicyDigest = policy.Name, policy.Severity, digest
		result.Policies[policy.Name] = state
		if state.Phase == "firing" || (state.Phase == "pending" && result.Phase != "firing") {
			result.Phase = state.Phase
		}
		if state.LastEvaluated != nil {
			value := *state.LastEvaluated
			result.LastEvaluated = &value
		}
		if event != nil {
			event.Policy, event.Severity = policy.Name, policy.Severity
			events = append(events, *event)
		}
	}
	return result, events
}
