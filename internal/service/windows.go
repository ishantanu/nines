package service

import (
	"context"
	"time"

	"github.com/ishantanu/nines/internal/model"
)

type WindowCollector interface {
	CollectWindows(context.Context, model.SLO, time.Time, time.Time) ([]model.WindowObservation, error)
}

func (s *Service) applyWindows(r *model.Record, observations []model.WindowObservation, from, to time.Time) error {
	if r.Windows == nil {
		r.Windows = map[string]model.WindowObservation{}
	}
	cutoff := s.Now().UTC().Truncate(time.Duration(r.SLO.StepSeconds) * time.Second).Add(-2 * time.Duration(r.SLO.WindowSeconds) * time.Second)
	allowed := map[int64]bool{}
	for _, w := range r.SLO.Windows() {
		allowed[w] = true
	}
	seen := map[string]bool{}
	for _, o := range observations {
		if err := o.Validate(r.SLO); err != nil {
			return err
		}
		key := model.WindowKey(o.WindowSeconds, o.Time)
		if !o.Time.After(from) || o.Time.After(to) || !o.Time.After(cutoff) || !allowed[o.WindowSeconds] || seen[key] {
			return model.Invalid("duplicate or out-of-range provider window")
		}
		seen[key] = true
	}
	for k, o := range r.Windows {
		if !o.Time.After(cutoff) || (o.Time.After(from) && !o.Time.After(to)) {
			delete(r.Windows, k)
		}
	}
	for _, o := range observations {
		r.Windows[model.WindowKey(o.WindowSeconds, o.Time)] = o
	}
	if len(r.Windows) > 800000 {
		return model.Invalid("provider-window observation limit exceeded")
	}
	// Replay only the bounded live overlap, in order. A 15-second worker can
	// cross two 10-second cutoffs; skipping one would reset a pending alert even
	// though every window was collected. Old backfills do not replay old alerts.
	at := s.recordEvaluationAt(*r, s.Now())
	step := time.Duration(r.SLO.StepSeconds) * time.Second
	grace := ((collectionInterval+step-1)/step + 1) * step
	start := at.Add(-grace)
	if start.Before(from.Add(step)) {
		start = from.Add(step)
	}
	for cursor := start; !cursor.After(at); cursor = cursor.Add(step) {
		if r.Alert.LastEvaluated != nil && !cursor.After(*r.Alert.LastEvaluated) && !cursor.Equal(at) {
			continue
		}
		s.advance(r, cursor)
	}
	return nil
}
