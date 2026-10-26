package systemone

import "errors"

// These local policy failures reveal no provider diagnostics or credentials.
var (
	ErrPaused            = errors.New("system_one_paused")
	ErrBudgetExhausted   = errors.New("system_one_budget_exhausted")
	ErrPolicyUnavailable = errors.New("system_one_policy_unavailable")
)
