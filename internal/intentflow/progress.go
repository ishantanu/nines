package intentflow

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	rm "github.com/ishantanu/nines/internal/reviewmodel"
)

// Model adapters invoke progress serially. Persist the first update, the final
// update and at most one intermediate update per second, under the worker lease.
func (s *Service) modelProgress(ctx context.Context, id, token string) func(rm.Progress) error {
	var last time.Time
	highest := 0
	return func(p rm.Progress) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p.Bytes < highest || p.Bytes < 0 || p.Bytes > 256<<10 {
			return rm.Failure("model_budget_exceeded")
		}
		highest = p.Bytes
		if !last.IsZero() && !p.Final && time.Since(last) < time.Second {
			return nil
		}
		_, err := s.Repository.Update(ctx, id, func(r *record, now time.Time) error {
			if err := fenced(*r, token, now); err != nil {
				return err
			}
			state := "receiving"
			if p.Final {
				state = "received"
			}
			run := current(r)
			run.Progress = &v1.IntentModelProgress{ModelCall: run.ModelCalls, State: state, Bytes: p.Bytes, At: now}
			// Transport observations do not invalidate the user's reviewed revision.
			return nil
		})
		if err == nil {
			last = time.Now()
		}
		return err
	}
}
