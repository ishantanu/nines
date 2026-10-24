package intentflow

import (
	"context"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/auditarchive"
)

type archiveState struct {
	Token                                string
	LeaseUntil, NextAttempt, RetainUntil time.Time
	Attempts                             int
	Receipt                              *v1.AuditArchiveReceipt
	LastError                            string
}

func (m *memory) ClaimArchive(ctx context.Context, tenant, target string, days int, token string) (auditarchive.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return auditarchive.Job{}, err
	}
	if m.archives == nil {
		m.archives = map[string]archiveState{}
	}
	now := time.Now().UTC()
	for _, item := range m.journal {
		if item.Tenant != tenant {
			continue
		}
		key := target + "/" + item.ID
		state := m.archives[key]
		if state.Receipt != nil || state.LeaseUntil.After(now) || state.NextAttempt.After(now) {
			continue
		}
		if !state.RetainUntil.After(now) {
			state.RetainUntil = now.Truncate(time.Second).AddDate(0, 0, days)
		}
		if err := validateJournalRecord(item); err != nil {
			return auditarchive.Job{}, err
		}
		state.Token = token
		state.LeaseUntil = now.Add(90 * time.Second)
		state.Attempts++
		m.archives[key] = state
		return auditarchive.Job{Record: clone(item), Token: token, RetainUntil: state.RetainUntil, Attempts: state.Attempts}, nil
	}
	return auditarchive.Job{}, auditarchive.ErrEmpty
}
func archiveDelay(attempts int) time.Duration {
	if attempts > 10 {
		attempts = 10
	}
	return min(time.Duration(1<<attempts)*5*time.Second, time.Hour)
}
func (m *memory) FinishArchive(ctx context.Context, tenant, target, id, token string, receipt *v1.AuditArchiveReceipt, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Tenant is checked against the immutable journal, never an externally supplied key.
	found := false
	for _, item := range m.journal {
		if item.ID == id && item.Tenant == tenant {
			found = true
			break
		}
	}
	key := target + "/" + id
	state := m.archives[key]
	if !found || state.Token != token || !state.LeaseUntil.After(time.Now()) {
		return auditarchive.ErrLease
	}
	state.Receipt = clone(receipt)
	state.LastError = code
	state.Token = ""
	state.LeaseUntil = time.Time{}
	state.NextAttempt = time.Now().UTC().Add(archiveDelay(state.Attempts))
	m.archives[key] = state
	return nil
}
func (m *memory) ArchiveStatus(ctx context.Context, tenant, target, id string) (v1.AuditArchiveStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := v1.AuditArchiveStatus{APIVersion: v1.APIVersion, Tenant: tenant, Enabled: true, Target: target, State: "pending", RecordID: id}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	found := false
	for _, item := range m.journal {
		if item.Tenant != tenant || id != "" && item.ID != id {
			continue
		}
		found = true
		state := m.archives[target+"/"+item.ID]
		if state.Receipt != nil {
			out.Verified++
			if state.Receipt.RetainUntil.After(time.Now()) {
				out.Retained++
			} else {
				out.Expired++
			}
			if id != "" {
				out.Receipt = clone(state.Receipt)
			}
		} else {
			out.Pending++
			if state.LastError != "" {
				out.Failed++
				out.LastError = state.LastError
			}
		}
	}
	if id != "" && !found {
		return out, ErrNotFound
	}
	if out.Pending == 0 {
		out.State = "verified"
	}
	if out.Failed > 0 {
		out.State = "retrying"
	}
	return out, nil
}
