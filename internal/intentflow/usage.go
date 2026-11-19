package intentflow

import (
	"context"
	"slices"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

const (
	usageDefaultDays = 90
	usageMaxDays     = 365
	// usageInspectCap bounds how many conversations one request reads.
	usageInspectCap = 500
)

// Usage summarizes intent workflow progress for the tenant. It only reads.
func (s *Service) Usage(ctx context.Context, days int) (v1.IntentUsage, error) {
	if days == 0 {
		days = usageDefaultDays
	}
	if days < 1 || days > usageMaxDays {
		return v1.IntentUsage{}, model.Invalid("days must be between 1 and 365")
	}
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)
	var sessions []v1.IntentConversation
	truncated := false
	after := ""
	for {
		rows, err := s.Repository.List(ctx, after, 51)
		if err != nil {
			return v1.IntentUsage{}, err
		}
		page := rows
		more := len(rows) > 50
		if more {
			page = rows[:50]
		}
		for _, row := range page {
			if row.CreatedAt.Before(from) {
				continue
			}
			if len(sessions) >= usageInspectCap {
				truncated = true
				break
			}
			rec, err := s.Repository.Read(ctx, row.ID)
			if err != nil {
				return v1.IntentUsage{}, err
			}
			if rec.Session.Tenant != s.Platform.TenantID {
				continue
			}
			sessions = append(sessions, rec.Session)
		}
		if !more || truncated {
			break
		}
		after = page[len(page)-1].ID
	}
	out := summarizeUsage(sessions, from, to)
	out.Tenant = s.Platform.TenantID
	out.WindowDays = days
	out.Truncated = truncated
	return out, nil
}

// summarizeUsage is pure so every stage definition can be tested directly.
func summarizeUsage(sessions []v1.IntentConversation, from, to time.Time) v1.IntentUsage {
	out := v1.IntentUsage{
		APIVersion: v1.APIVersion, From: from, To: to, Inspected: len(sessions), Started: len(sessions),
		Limitations: []string{
			"Counts describe workflow progress for conversations created in the window. They do not show whether an SLO is healthy or correct.",
			"Applied-first-proposal means the first proposal was the one applied; it does not measure the quality of the measurement.",
			"Medians use server timestamps and include time spent waiting for review.",
		},
	}
	var toProposal, toApplied []int64
	for _, session := range sessions {
		sets := append([]v1.IntentChangeSet{}, session.ChangeSetHistory...)
		if session.ChangeSet != nil && !slices.ContainsFunc(sets, func(c v1.IntentChangeSet) bool { return c.Digest == session.ChangeSet.Digest }) {
			sets = append(sets, *session.ChangeSet)
		}
		slices.SortFunc(sets, func(a, b v1.IntentChangeSet) int { return a.CreatedAt.Compare(b.CreatedAt) })
		for _, run := range session.Runs {
			switch run.Mode {
			case "guided":
				out.GuidedRuns++
			case "model":
				out.ModelRuns++
			}
		}
		if len(sets) == 0 {
			continue
		}
		out.Proposed++
		toProposal = append(toProposal, int64(sets[0].CreatedAt.Sub(session.CreatedAt).Seconds()))
		var approved, rejected, rolledBack, backtest bool
		var firstApplied *time.Time
		for i, c := range sets {
			approved = approved || c.ApprovedAt != nil
			rejected = rejected || c.RejectedAt != nil
			rolledBack = rolledBack || rollbackApplied(c)
			backtest = backtest || (c.Recommendation != nil && c.Recommendation.Backtest != nil)
			if c.AppliedAt != nil && (firstApplied == nil || c.AppliedAt.Before(*firstApplied)) {
				at := *c.AppliedAt
				firstApplied = &at
				if i == 0 {
					out.AppliedFirstProposal++
				}
			}
		}
		if approved {
			out.Approved++
		}
		if rejected {
			out.Rejected++
		}
		if rolledBack {
			out.RolledBack++
		}
		if backtest {
			out.BacktestAttached++
		}
		if firstApplied != nil {
			out.Applied++
			toApplied = append(toApplied, int64(firstApplied.Sub(session.CreatedAt).Seconds()))
		}
	}
	out.MedianSecondsToProposal = median(toProposal)
	out.MedianSecondsToApplied = median(toApplied)
	return out
}

func median(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	slices.Sort(values)
	m := values[len(values)/2]
	if len(values)%2 == 0 {
		m = (values[len(values)/2-1] + values[len(values)/2]) / 2
	}
	return &m
}

// rollbackApplied is true only for a rollback that was applied, not proposed.
func rollbackApplied(c v1.IntentChangeSet) bool {
	if c.Rollback != nil && c.Rollback.AppliedAt != nil {
		return true
	}
	return slices.ContainsFunc(c.RollbackHistory, func(r v1.IntentRollback) bool { return r.AppliedAt != nil })
}
