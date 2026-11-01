package intentflow

import (
	"context"
	"time"

	"github.com/ishantanu/nines/internal/workflowhook"
)

type workflowEventState struct {
	Token, LastError        string
	LeaseUntil, NextAttempt time.Time
	Attempts                int
	Delivered               bool
}

func (m *memory) ClaimWorkflowEvent(ctx context.Context, tenant, target string, since time.Time, token string) (workflowhook.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return workflowhook.Job{}, err
	}
	if m.notifications == nil {
		m.notifications = map[string]workflowEventState{}
	}
	now := time.Now().UTC()
	for _, item := range m.journal {
		if item.Tenant != tenant || item.Event.At.Before(since) || !workflowhook.Eligible(item) {
			continue
		}
		k := tenant + "/" + target + "/" + item.ID
		state := m.notifications[k]
		if state.Delivered || state.LeaseUntil.After(now) || state.NextAttempt.After(now) {
			continue
		}
		if err := validateJournalRecord(item); err != nil {
			return workflowhook.Job{}, err
		}
		state.Token = token
		state.LeaseUntil = now.Add(90 * time.Second)
		state.Attempts++
		m.notifications[k] = state
		return workflowhook.Job{Record: clone(item), Token: token, Attempts: state.Attempts}, nil
	}
	return workflowhook.Job{}, workflowhook.ErrEmpty
}
func (m *memory) FinishWorkflowEvent(ctx context.Context, tenant, target, id, token string, delivered bool, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	k := tenant + "/" + target + "/" + id
	state, ok := m.notifications[k]
	if !ok || state.Token != token || !state.LeaseUntil.After(time.Now()) {
		return workflowhook.ErrLease
	}
	state.Delivered = delivered
	state.LastError = code
	state.Token = ""
	state.LeaseUntil = time.Time{}
	state.NextAttempt = time.Now().UTC().Add(archiveDelay(state.Attempts))
	m.notifications[k] = state
	return nil
}
