package intentflow

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestConversationCapacityRecoveryPreservesHistory(t *testing.T) {
	for _, pg := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "postgres"}[pg], func(t *testing.T) {
			s, ctx, _ := fixture(t, pg)
			original, err := s.Create(ctx, input())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Repository.Update(ctx, original.ID, func(r *record, _ time.Time) error { r.Session.Runs = make([]v1.IntentFlowRun, maxRuns); return nil })
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Start(ctx, original.ID, v1.CreateIntentFlowRun{ID: "one-too-many", ExpectedVersion: original.Version, Mode: "guided"})
			var capacity *CapacityError
			if !errors.Is(err, ErrLimit) || !errors.As(err, &capacity) || capacity.Resource != "preparation_runs" {
				t.Fatal("opaque or missing capacity reason", err)
			}
			w := httptest.NewRecorder()
			fail(w, err)
			var payload struct {
				Code  string `json:"code"`
				Limit int    `json:"limit"`
			}
			if e := json.Unmarshal(w.Body.Bytes(), &payload); e != nil || w.Code != 429 || payload.Code != "intent_preparation_runs_limit" || payload.Limit != maxRuns {
				t.Fatal(w.Code, w.Body.String(), e)
			}
			next := input()
			next.ID = "conversation-next"
			created, err := s.Create(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			if created.Goal == nil || len(created.Runs) != 0 || created.ChangeSet != nil || created.Selection != nil {
				t.Fatal("new conversation inherited evidence or approvals", created)
			}
			retained, err := s.Read(ctx, original.ID)
			if err != nil || len(retained.Runs) != maxRuns || retained.Version != original.Version || len(retained.Audit) != len(original.Audit) {
				t.Fatal("capacity failure rewrote history", retained, err)
			}
		})
	}
}
