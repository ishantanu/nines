package store

import "github.com/ishantanu/nines/internal/model"

func cloneAlertPolicies(source map[string]model.AlertState) map[string]model.AlertState {
	if source == nil {
		return nil
	}
	out := make(map[string]model.AlertState, len(source))
	for id, state := range source {
		if state.PendingSince != nil {
			at := *state.PendingSince
			state.PendingSince = &at
		}
		if state.LastEvaluated != nil {
			at := *state.LastEvaluated
			state.LastEvaluated = &at
		}
		state.Policies = nil
		out[id] = state
	}
	return out
}
