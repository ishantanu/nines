package intentflow

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func at(base time.Time, minutes int) *time.Time {
	t := base.Add(time.Duration(minutes) * time.Minute)
	return &t
}

func TestSummarizeUsageStagesAndMedians(t *testing.T) {
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	mk := func(id string, sets ...v1.IntentChangeSet) v1.IntentConversation {
		return v1.IntentConversation{IntentConversationSummary: v1.IntentConversationSummary{ID: id, CreatedAt: base}, ChangeSetHistory: sets}
	}
	applied := func(digest string, created, appliedMin int) v1.IntentChangeSet {
		return v1.IntentChangeSet{Digest: digest, CreatedAt: *at(base, created), ApprovedAt: at(base, created+1), AppliedAt: at(base, appliedMin)}
	}
	sessions := []v1.IntentConversation{
		mk("only-draft"),
		mk("first-try", applied("a", 10, 30)),
		mk("reworked",
			v1.IntentChangeSet{Digest: "b1", CreatedAt: *at(base, 20), RejectedAt: at(base, 25)},
			applied("b2", 40, 70)),
		{IntentConversationSummary: v1.IntentConversationSummary{ID: "pending", CreatedAt: base},
			ChangeSet: &v1.IntentChangeSet{Digest: "c", CreatedAt: *at(base, 5), Recommendation: &v1.IntentRecommendation{Backtest: &v1.IntentBacktestSummary{}}}},
	}
	sessions[0].Runs = []v1.IntentFlowRun{{Mode: "guided"}, {Mode: "model"}, {Mode: "model"}}
	sessions[1].ChangeSetHistory[0].RollbackHistory = []v1.IntentRollback{{AppliedAt: at(base, 90)}}
	sessions[2].ChangeSetHistory[1].Rollback = &v1.IntentRollback{} // proposed, not applied
	got := summarizeUsage(sessions, base.AddDate(0, 0, -1), base.AddDate(0, 0, 1))
	want := map[string][2]int{
		"started": {got.Started, 4}, "proposed": {got.Proposed, 3}, "approved": {got.Approved, 2},
		"applied": {got.Applied, 2}, "rejected": {got.Rejected, 1}, "appliedFirstProposal": {got.AppliedFirstProposal, 1},
		"rolledBack": {got.RolledBack, 1}, "backtest": {got.BacktestAttached, 1},
		"guided": {got.GuidedRuns, 1}, "model": {got.ModelRuns, 2},
	}
	for name, v := range want {
		if v[0] != v[1] {
			t.Errorf("%s = %d, want %d", name, v[0], v[1])
		}
	}
	// Proposals at 10, 20 and 5 minutes: median 10. Applied at 30 and 70: median 50.
	if got.MedianSecondsToProposal == nil || *got.MedianSecondsToProposal != 600 {
		t.Errorf("median to proposal = %v", got.MedianSecondsToProposal)
	}
	if got.MedianSecondsToApplied == nil || *got.MedianSecondsToApplied != 3000 {
		t.Errorf("median to applied = %v", got.MedianSecondsToApplied)
	}
	if len(got.Limitations) == 0 {
		t.Error("limitations must accompany every summary")
	}
}

func TestSummarizeUsageEmptyHasNoMedians(t *testing.T) {
	got := summarizeUsage(nil, time.Now().AddDate(0, 0, -1), time.Now())
	if got.Started != 0 || got.MedianSecondsToProposal != nil || got.MedianSecondsToApplied != nil {
		t.Fatalf("empty summary = %+v", got)
	}
}
