package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ishantanu/nines/internal/model"
)

// DefinitionBatchStore runs bounded, database-only definition work atomically.
// A committed receipt makes a retry recoverable after conversation persistence fails.
// With commit=false all work is rolled back, including alerts and history rows.
type DefinitionBatchStore interface {
	DefinitionBatch(context.Context, string, string, bool, func(Store) (json.RawMessage, error)) (json.RawMessage, error)
}
type definitionReceipt struct {
	Digest string          `json:"digest"`
	Result json.RawMessage `json:"result"`
}

func (m *Memory) DefinitionBatch(ctx context.Context, key, digest string, commit bool, fn func(Store) (json.RawMessage, error)) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if old, ok := m.batches[key]; commit && ok {
		if old.Digest != digest {
			return nil, model.ErrRevisionConflict
		}
		return append(json.RawMessage{}, old.Result...), nil
	}
	snapshot := NewMemory()
	snapshot.records = copyJSON(m.records)
	snapshot.archives = copyJSON(m.archives)
	snapshot.deliveries = copyJSON(m.deliveries)
	snapshot.topology = copyJSON(m.topology)
	snapshot.journeys = copyJSON(m.journeys)
	snapshot.receipts = copyJSON(m.receipts)
	snapshot.runs = copyJSON(m.runs)
	result, err := fn(snapshot)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("invalid definition receipt")
	}
	if commit {
		m.records = snapshot.records
		m.archives = snapshot.archives
		m.deliveries = snapshot.deliveries
		m.topology = snapshot.topology
		m.journeys = snapshot.journeys
		m.receipts = snapshot.receipts
		m.runs = snapshot.runs
		if m.batches == nil {
			m.batches = map[string]definitionReceipt{}
		}
		m.batches[key] = definitionReceipt{digest, append(json.RawMessage{}, result...)}
	}
	return append(json.RawMessage{}, result...), nil
}

// Pool and transaction adapters share the existing store implementation; nested
// store operations use savepoints and cannot commit the surrounding batch.
type postgresDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	Ping(context.Context) error
	Close()
}
type transactionDB struct{ pgx.Tx }

func (t transactionDB) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return t.Begin(ctx)
}
func (t transactionDB) Ping(ctx context.Context) error {
	_, err := t.Exec(ctx, "SELECT 1")
	return err
}
func (t transactionDB) Close() {}
func (p *Postgres) DefinitionBatch(ctx context.Context, key, digest string, commit bool, fn func(Store) (json.RawMessage, error)) (json.RawMessage, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	// Serialize with every definition writer, including direct API/CLI writes from
	// another replica. No network work is allowed while holding these tenant tables.
	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='5s'; LOCK TABLE slos, journey_revisions, topology_revisions, definition_batch_receipts IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return nil, err
	}
	if commit {
		var b []byte
		err = tx.QueryRow(ctx, "SELECT document FROM definition_batch_receipts WHERE operation_id=$1", key).Scan(&b)
		if err == nil {
			var old definitionReceipt
			if err = json.Unmarshal(b, &old); err != nil {
				return nil, err
			}
			if old.Digest != digest {
				return nil, model.ErrRevisionConflict
			}
			return old.Result, tx.Commit(ctx)
		}
		if err != pgx.ErrNoRows {
			return nil, err
		}
	}
	result, err := fn(&Postgres{pool: transactionDB{tx}})
	if err != nil {
		return nil, err
	}
	if !json.Valid(result) {
		return nil, fmt.Errorf("invalid definition receipt")
	}
	if !commit {
		return result, nil
	}
	b, err := json.Marshal(definitionReceipt{digest, result})
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO definition_batch_receipts(operation_id,document)VALUES($1,$2)", key, b); err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}
