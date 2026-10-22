package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ishantanu/nines/internal/model"
)

// JourneyApplier supplies an atomic, durable idempotency receipt for approved
// operations. Receipts are scoped to the same tenant schema as journeys.
type JourneyApplier interface {
	ApplyJourneyOnce(context.Context, model.JourneyRevision, int64, string) (model.JourneyRevision, error)
}
type journeyReceipt struct {
	Expected int64                 `json:"expected"`
	Result   model.JourneyRevision `json:"result"`
}

func receiptMatches(r journeyReceipt, next model.JourneyRevision, expected int64) bool {
	return r.Expected == expected && r.Result.Manifest.Metadata.Name == next.Manifest.Metadata.Name && r.Result.Digest == next.Digest
}
func validateOperation(next model.JourneyRevision, expected int64, operation string) error {
	if expected < 0 || operation == "" || len(operation) > 128 || next.Digest != model.JourneyDigest(next.Manifest) {
		return model.Invalid("invalid journey operation, revision or manifest digest")
	}
	return next.Manifest.Validate()
}
func (m *Memory) ApplyJourneyOnce(ctx context.Context, next model.JourneyRevision, expected int64, operation string) (model.JourneyRevision, error) {
	if err := validateOperation(next, expected, operation); err != nil {
		return next, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return next, err
	}
	if old, ok := m.receipts[operation]; ok {
		if !receiptMatches(old, next, expected) {
			return model.JourneyRevision{}, model.ErrRevisionConflict
		}
		return copyJSON(old.Result), nil
	}
	history := m.journeys[next.Manifest.Metadata.Name]
	var current model.JourneyRevision
	if len(history) > 0 {
		current = history[len(history)-1]
	}
	if current.Revision != expected {
		return current, model.ErrRevisionConflict
	}
	if current.Digest == next.Digest {
		next = current
	} else {
		next.Revision = expected + 1
		if m.journeys == nil {
			m.journeys = map[string][]model.JourneyRevision{}
		}
		m.journeys[next.Manifest.Metadata.Name] = append(history, copyJSON(next))
	}
	if m.receipts == nil {
		m.receipts = map[string]journeyReceipt{}
	}
	m.receipts[operation] = journeyReceipt{expected, copyJSON(next)}
	return copyJSON(next), nil
}
func (p *Postgres) ApplyJourneyOnce(ctx context.Context, next model.JourneyRevision, expected int64, operation string) (model.JourneyRevision, error) {
	if err := validateOperation(next, expected, operation); err != nil {
		return next, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return next, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':' || $1,654239121))", operation); err != nil {
		return next, err
	}
	var b []byte
	err = tx.QueryRow(ctx, "SELECT document FROM journey_apply_receipts WHERE operation_id=$1", operation).Scan(&b)
	if err == nil {
		var receipt journeyReceipt
		if err = json.Unmarshal(b, &receipt); err != nil {
			return next, err
		}
		if !receiptMatches(receipt, next, expected) {
			return model.JourneyRevision{}, model.ErrRevisionConflict
		}
		return receipt.Result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return next, err
	}
	id := next.Manifest.Metadata.Name
	// Use the identical journey lock as ordinary ApplyJourney, including creates.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':' || $1,654239119))", id); err != nil {
		return next, err
	}
	var current model.JourneyRevision
	err = tx.QueryRow(ctx, "SELECT document FROM journey_revisions WHERE journey_id=$1 ORDER BY revision DESC LIMIT 1", id).Scan(&b)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return next, err
	}
	if err == nil {
		if err = json.Unmarshal(b, &current); err != nil {
			return next, err
		}
	}
	if current.Revision != expected {
		return current, model.ErrRevisionConflict
	}
	if current.Digest == next.Digest {
		next = current
	} else {
		next.Revision = expected + 1
		b, err = json.Marshal(next)
		if err != nil {
			return next, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO journey_revisions(journey_id,revision,document) VALUES($1,$2,$3)", id, next.Revision, b); err != nil {
			return next, err
		}
	}
	b, err = json.Marshal(journeyReceipt{expected, next})
	if err != nil {
		return next, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO journey_apply_receipts(operation_id,document) VALUES($1,$2)", operation, b); err != nil {
		return next, err
	}
	return next, tx.Commit(ctx)
}
