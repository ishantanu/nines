package service

import (
	"context"
	"time"

	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
)

func (s *Service) History(ctx context.Context, id string, from, to time.Time, resolution int64, requestedRange ...int64) (engine.History, error) {
	return s.HistoryForPolicy(ctx, id, from, to, resolution, "", requestedRange...)
}
func (s *Service) HistoryForPolicy(ctx context.Context, id string, from, to time.Time, resolution int64, policyName string, requestedRange ...int64) (engine.History, error) {
	r, err := s.Store.Read(ctx, id)
	if err != nil {
		return engine.History{}, err
	}
	return s.historyFromRecord(ctx, r, s.Now(), from, to, resolution, policyName, requestedRange...)
}
func (s *Service) historyFromRecord(ctx context.Context, r model.Record, now, from, to time.Time, resolution int64, policyName string, requestedRange ...int64) (engine.History, error) {
	if len(r.SLO.Policies) > 0 {
		found := false
		for _, p := range r.SLO.Policies {
			if policyName == "" || p.Name == policyName {
				copy := p
				r.SLO.Policy = &copy
				found = true
				break
			}
		}
		if !found {
			return engine.History{}, model.Invalid("unknown alert policy")
		}
	} else if policyName != "" {
		return engine.History{}, model.Invalid("unknown alert policy")
	}
	current := s.recordEvaluationAt(r, now)
	if to.IsZero() {
		to = current
	}
	if from.IsZero() {
		seconds := min(int64(86400), 2*r.SLO.WindowSeconds)
		if len(requestedRange) > 0 {
			seconds = requestedRange[0]
		}
		from = to.Add(-time.Duration(seconds) * time.Second).Truncate(time.Duration(r.SLO.StepSeconds) * time.Second)
	}
	if !aligned(from, r.SLO.StepSeconds) || !aligned(to, r.SLO.StepSeconds) || !to.After(from) || to.After(current) || to.Sub(from) > time.Duration(2*r.SLO.WindowSeconds)*time.Second || from.Before(current.Add(-time.Duration(2*r.SLO.WindowSeconds)*time.Second)) {
		return engine.History{}, model.Invalid("history requires aligned from < to within two retained SLO windows, ending at or before the current evaluation")
	}
	if resolution == 0 {
		resolution = max(r.SLO.StepSeconds, ((int64(to.Sub(from)/time.Second)/300+r.SLO.StepSeconds-1)/r.SLO.StepSeconds)*r.SLO.StepSeconds)
	}
	if resolution > 2*r.SLO.WindowSeconds || resolution < r.SLO.StepSeconds || resolution%r.SLO.StepSeconds != 0 || int64(to.Sub(from)/time.Second)/resolution+1 > 1500 {
		return engine.History{}, model.Invalid("resolution must be a multiple of stepSeconds with at most 1500 chart points")
	}
	result := engine.HistoryRecord(r, from, to, resolution)
	events, err := s.Store.Deliveries(ctx)
	if err != nil {
		return result, err
	}
	for _, event := range events {
		if event.Event.SLOID == r.SLO.ID && (event.Event.ConfigRevision == 0 || event.Event.ConfigRevision == model.ConfigRevision(r.SLO)) && !event.Event.Time.Before(from) && !event.Event.Time.After(to) {
			result.AlertEvents = append(result.AlertEvents, event.Event)
		}
	}
	return result, nil
}
