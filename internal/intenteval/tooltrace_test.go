package intenteval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

func TestToolTracePreservesResponsesAndBoundsSanitizedDiagnostics(t *testing.T) {
	const secret = "private-provider-secret"
	trace := &toolTrace{Model: selectionTestModel{call: rm.Call{Name: secret, ID: secret, Arguments: json.RawMessage(`{"secret":"private-provider-secret"}`)}}}
	for i := 0; i < 20; i++ {
		response, err := trace.Turn(context.Background(), rm.Request{}, nil)
		if err != nil || len(response.Calls) != 1 || response.Calls[0].Name != secret {
			t.Fatal("trace altered provider response")
		}
	}
	if trace.calls != 20 || trace.attempts != 20 || len(trace.sequence) != 16 {
		t.Fatal("diagnostics not bounded", trace)
	}
	raw, err := json.Marshal(Result{ToolSequence: trace.sequence})
	if err != nil || strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "unavailable_tool") {
		t.Fatal("unsafe diagnostic", err)
	}
	trace = &toolTrace{Model: selectionTestModel{call: rm.Call{Name: "list_slo_templates"}}}
	if _, err = trace.Turn(context.Background(), rm.Request{}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(trace.sequence, []string{"list_slo_templates"}) {
		t.Fatal(trace.sequence)
	}
}
