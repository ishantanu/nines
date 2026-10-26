package intenteval

import (
	"context"

	"testing"
)

func TestTemplateEvaluationContracts(t *testing.T) {
	report, err := Templates(context.Background())
	if err != nil {
		t.Fatal(err, report.Results)
	}
	if !report.Complete || report.Mode != "deterministic" || report.Suite != "templates" || report.Failed != 0 || report.Passed != 120 || len(report.SuiteSHA256) != 64 {
		t.Fatal(report)
	}
	again, err := Templates(context.Background())
	if err != nil || again.SuiteSHA256 != report.SuiteSHA256 {
		t.Fatal("unstable suite digest", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, err := Templates(ctx)
	if err == nil || cancelled.Complete {
		t.Fatal("cancellation reported complete")
	}
}
