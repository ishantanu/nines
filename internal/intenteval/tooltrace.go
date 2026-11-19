package intenteval

import (
	"context"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

// toolTrace records only fixed tool identifiers, never provider-owned names,
// arguments, call IDs, text or native responses. The cap also bounds bad models
// returning multiple calls before the production worker rejects the response.
type toolTrace struct {
	rm.Model
	calls, attempts int
	sequence        []string
}

func (t *toolTrace) Turn(ctx context.Context, request rm.Request, progress func(rm.Progress) error) (rm.Response, error) {
	t.calls++
	response, err := t.Model.Turn(ctx, request, progress)
	for _, call := range response.Calls {
		t.attempts++
		if len(t.sequence) >= 16 {
			continue
		}
		name := "unavailable_tool"
		switch call.Name {
		case "discover_measurement_candidates", "draft_goal", "list_slo_templates", "render_slo_template", "find_measurements", "compile_intent", "backtest_intent", "ask_user", "finish_proposal":
			name = call.Name
		}
		t.sequence = append(t.sequence, name)
	}
	return response, err
}
