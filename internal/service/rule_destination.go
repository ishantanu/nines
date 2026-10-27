package service

import (
	"context"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
)

// PinRuleDestination runs immediately before publication and persists the
// trusted destination without credentials. It survives source-file edits and
// ambiguous provider acknowledgements. Retirement keeps this pin until deletion.
func (s *Service) PinRuleDestination(ctx context.Context, slo model.SLO, target rules.Target) error {
	digest := rules.RouteDigest(target)
	return s.Store.Update(ctx, slo.ID, func(r *model.Record) error {
		if r.SLO.Retired() || model.ConfigRevision(r.SLO) != model.ConfigRevision(slo) {
			return model.ErrRevisionConflict
		}
		if r.SLO.RuleDestination != "" && r.SLO.RuleDestination != digest {
			return model.Invalid("original recording-rule destination is pinned; withdraw owned rules before changing routes")
		}
		r.SLO.RuleDestination = digest
		return nil
	})
}

// ClearRuleDestination is called only after reviewed owned withdrawal succeeds.
func (s *Service) ClearRuleDestination(ctx context.Context, id string, revision int64) error {
	return s.Store.Update(ctx, id, func(r *model.Record) error {
		if r.SLO.Retired() || model.ConfigRevision(r.SLO) != revision {
			return model.ErrRevisionConflict
		}
		r.SLO.RuleDestination = ""
		return nil
	})
}
