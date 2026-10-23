package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// RuleAutomationReader avoids loading retained samples during scheduler scans.
type RuleAutomationReader interface {
	RuleAutomation(context.Context, string) (*v1.RuleAutomationStatus, error)
}

func (m *Memory) RuleAutomation(ctx context.Context, id string) (*v1.RuleAutomationStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	r, ok := m.records[id]
	if !ok {
		return nil, model.ErrNotFound
	}
	if r.RuleAutomation == nil {
		return nil, nil
	}
	v := copyJSON(*r.RuleAutomation)
	return &v, nil
}
func (p *Postgres) RuleAutomation(ctx context.Context, id string) (*v1.RuleAutomationStatus, error) {
	var raw []byte
	e := p.pool.QueryRow(ctx, "SELECT rule_automation FROM slos WHERE id=$1", id).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if e != nil || len(raw) == 0 {
		return nil, e
	}
	var out *v1.RuleAutomationStatus
	e = json.Unmarshal(raw, &out)
	return out, e
}
