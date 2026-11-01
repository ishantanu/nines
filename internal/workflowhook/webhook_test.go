package workflowhook

import (
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestWebhookConfigurationRequiresTrustedEndpointAndKey(t *testing.T) {
	key := "01234567890123456789012345678901"
	since := time.Now()
	for _, url := range []string{"http://external.example/path", "https://user:pass@example.com/", "https://example.com/?token=secret", "file:///tmp/hook"} {
		if Validate(url, key, since) == nil {
			t.Fatal("invalid endpoint", url)
		}
	}
	if Validate("http://127.0.0.1:9999/hook", key, since) != nil || Validate("https://example.com/hook", key, since) != nil {
		t.Fatal("valid endpoint rejected")
	}
	if Validate("https://example.com/hook", "short", since) == nil || Validate("https://example.com/hook", key, time.Time{}) == nil || Validate("https://example.com/hook", key+"\n", since) == nil {
		t.Fatal("invalid key or cutoff accepted")
	}
}

func TestEligibleWorkflowEvents(t *testing.T) {
	for _, action := range []string{"change_set.proposed", "change_set.approved", "change_set.applied", "change_set.rejected", "rollback.proposed", "rollback.approved", "rollback.applied", "rollback.rejected"} {
		for _, outcome := range []string{"succeeded", "blocked"} {
			r := v1.AuditRecord{Event: v1.IntentAuditEvent{Action: action, Outcome: outcome}}
			if !Eligible(r) {
				t.Errorf("expected eligible: %s %s", action, outcome)
			}
			r.Event.Outcome = "denied"
			if Eligible(r) {
				t.Errorf("unexpected denied event: %s", action)
			}
		}
	}
	for _, action := range []string{"change_set.observed", "rollback.observed"} {
		r := v1.AuditRecord{Event: v1.IntentAuditEvent{Action: action, Outcome: "succeeded"}}
		if Eligible(r) {
			t.Fatal("missing observation accepted")
		}
		r.Outcome = &v1.IntentOutcome{State: "pending"}
		if Eligible(r) {
			t.Fatal("pending observation accepted")
		}
		r.Outcome.State = "verified"
		if !Eligible(r) {
			t.Fatal("terminal observation rejected")
		}
	}
}
