package intenteval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNativeProviderFullWorkflowContracts(t *testing.T) {
	report, err := WorkflowFixtures(context.Background())
	if err != nil {
		for _, result := range report.Results {
			if !result.Passed {
				t.Logf("%s %s: %s (%s)", result.Provider, result.Case, result.Failure, result.Status)
			}
		}
		t.Fatal(err)
	}
	if !report.Complete || report.Suite != "workflow" || report.Passed != 60 || report.Failed != 0 {
		t.Fatalf("unexpected workflow report: %+v", report)
	}
}

func TestWorkflowCaseSelection(t *testing.T) {
	report, err := WorkflowFixtures(context.Background(), "availability-reviewed-apply")
	if err != nil {
		t.Fatal(err)
	}
	if report.SelectedCase != "availability-reviewed-apply" || !report.Complete || report.Passed != 5 || report.Failed != 0 {
		t.Fatalf("filtered report: %+v", report)
	}
	for _, result := range report.Results {
		if result.Case != report.SelectedCase {
			t.Fatalf("unselected case ran: %s", result.Case)
		}
	}
	unknown, err := WorkflowFixtures(context.Background(), "does-not-exist")
	if err == nil || unknown.Complete || len(unknown.Results) != 0 {
		t.Fatalf("unknown case ran: %+v, %v", unknown, err)
	}
}

func TestLiveScriptedCaseCannotReportAcceptance(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected model call", 500)
	}))
	defer server.Close()
	env := map[string]string{"INTENT_MODEL_PROVIDER": "custom", "INTENT_MODEL_MODEL": "fixture-model", "INTENT_MODEL_ENDPOINT": server.URL + "/v1/chat/completions"}
	report, err := WorkflowLive(context.Background(), func(key string) string { return env[key] }, "fixture", "reject-provider-routing-tool")
	if err == nil || !report.Complete || len(report.Results) != 0 || len(report.Skipped) != 1 || calls.Load() != 0 {
		t.Fatalf("scripted-only live case accepted: %+v, %v; requests: %d", report, err, calls.Load())
	}
}
