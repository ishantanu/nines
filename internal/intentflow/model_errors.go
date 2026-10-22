package intentflow

import (
	"errors"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

// Preserve actionable adapter failures without allowing arbitrary provider text
// or custom error codes into transcripts and public error fields.
func safeModelFailure(err error) string {
	var failure *rm.Error
	if errors.As(err, &failure) {
		switch failure.Code {
		case "model_unavailable", "model_capability_unavailable", "model_budget_exceeded", "model_incomplete", "model_invalid_response", "model_refused", "model_truncated", "unsafe_tool_call":
			return failure.Code
		}
	}
	return "model_unavailable"
}
