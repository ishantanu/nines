package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/observability"
)

//go:embed schema.sql
var schema string

type Postgres struct{ pool postgresDB }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	p, err := observability.NewPool(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	// A transaction-scoped advisory lock serializes bootstrap DDL across replicas.
	tx, err := p.Begin(ctx)
	if err != nil {
		p.Close()
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(654239118)"); err == nil {
		_, err = tx.Exec(ctx, schema)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		p.Close()
		return nil, err
	}
	return &Postgres{p}, nil
}
func (p *Postgres) Create(ctx context.Context, s model.SLO) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, "INSERT INTO slos(id,config) VALUES($1,$2)", s.ID, b)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return model.ErrConflict
	}
	return err
}
func (p *Postgres) List(ctx context.Context) ([]model.SLO, error) {
	rows, err := p.pool.Query(ctx, "SELECT config FROM slos ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SLO{}
	for rows.Next() {
		var b []byte
		var s model.SLO
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func load(ctx context.Context, tx pgx.Tx, id string, lock bool) (model.Record, error) {
	query := "SELECT config,alert_state,collection_health,operator_route FROM slos WHERE id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	var config, state, collection []byte
	r := model.Record{Points: map[int64]model.Point{}, Windows: map[string]model.WindowObservation{}}
	err := tx.QueryRow(ctx, query, id).Scan(&config, &state, &collection, &r.OperatorRoute)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(config, &r.SLO); err != nil {
		return r, err
	}
	if err = json.Unmarshal(collection, &r.Collection); err != nil {
		return r, err
	}
	if err = json.Unmarshal(state, &r.Alert); err != nil {
		return r, err
	}
	rows, err := tx.Query(ctx, "SELECT time_seconds,good,total FROM samples WHERE slo_id=$1 ORDER BY time_seconds", id)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var k int64
		var v model.Point
		if err = rows.Scan(&k, &v.Good, &v.Total); err != nil {
			return r, err
		}
		v.Time = time.Unix(k, 0).UTC()
		r.Points[k] = v
	}
	if err = rows.Err(); err != nil {
		return r, err
	}
	rows.Close()
	windows, err := tx.Query(ctx, "SELECT window_seconds,time_seconds,revision,good,total,coverage FROM window_observations WHERE slo_id=$1", id)
	if err != nil {
		return r, err
	}
	defer windows.Close()
	for windows.Next() {
		var o model.WindowObservation
		var at int64
		if err = windows.Scan(&o.WindowSeconds, &at, &o.Revision, &o.Good, &o.Total, &o.Coverage); err != nil {
			return r, err
		}
		o.Time = time.Unix(at, 0).UTC()
		r.Windows[model.WindowKey(o.WindowSeconds, o.Time)] = o
	}
	return r, windows.Err()
}
func (p *Postgres) Read(ctx context.Context, id string) (model.Record, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.Record{}, err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	r, err := load(ctx, tx, id, false)
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (p *Postgres) Update(ctx context.Context, id string, fn func(*model.Record) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	r, err := load(ctx, tx, id, true)
	if err != nil {
		return err
	}
	before := clone(r)
	if err = fn(&r); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for k, v := range r.Points {
		old, ok := before.Points[k]
		if !ok || old.Good != v.Good || old.Total != v.Total {
			batch.Queue("INSERT INTO samples(slo_id,time_seconds,good,total) VALUES($1,$2,$3,$4) ON CONFLICT(slo_id,time_seconds) DO UPDATE SET good=excluded.good,total=excluded.total", id, k, v.Good, v.Total)
		}
	}
	for k := range before.Points {
		if _, ok := r.Points[k]; !ok {
			batch.Queue("DELETE FROM samples WHERE slo_id=$1 AND time_seconds=$2", id, k)
		}
	}
	for k, o := range r.Windows {
		old, ok := before.Windows[k]
		if !ok || old != o {
			batch.Queue("INSERT INTO window_observations(slo_id,window_seconds,time_seconds,revision,good,total,coverage) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(slo_id,window_seconds,time_seconds) DO UPDATE SET revision=excluded.revision,good=excluded.good,total=excluded.total,coverage=excluded.coverage", id, o.WindowSeconds, o.Time.Unix(), o.Revision, o.Good, o.Total, o.Coverage)
		}
	}
	for k, o := range before.Windows {
		if _, ok := r.Windows[k]; !ok {
			batch.Queue("DELETE FROM window_observations WHERE slo_id=$1 AND window_seconds=$2 AND time_seconds=$3", id, o.WindowSeconds, o.Time.Unix())
		}
	}
	if batch.Len() > 0 {
		results := tx.SendBatch(ctx, batch)
		if err = results.Close(); err != nil {
			return err
		}
	}
	state, err := json.Marshal(r.Alert)
	if err != nil {
		return err
	}
	config, err := json.Marshal(r.SLO)
	if err != nil {
		return err
	}
	collection, err := json.Marshal(r.Collection)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE slos SET alert_state=$2,config=$3,collection_health=$4,operator_route=$5 WHERE id=$1", id, state, config, collection, r.OperatorRoute); err != nil {
		return err
	}
	for _, e := range r.Events {
		body, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO alert_outbox(id,event,next_attempt) VALUES($1,$2,$3)", e.ID, body, e.Time); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Deliveries(ctx context.Context) ([]model.Delivery, error) {
	rows, err := p.pool.Query(ctx, "SELECT event,status,attempts,next_attempt,last_error FROM alert_outbox ORDER BY (event->>'time') DESC,id LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Delivery{}
	for rows.Next() {
		var d model.Delivery
		var b []byte
		if err = rows.Scan(&b, &d.Status, &d.Attempts, &d.NextAttempt, &d.LastError); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &d.Event); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (p *Postgres) Claim(ctx context.Context, now time.Time, lease time.Duration) (*model.Delivery, error) {
	var d model.Delivery
	var b []byte
	err := p.pool.QueryRow(ctx, `UPDATE alert_outbox SET attempts=attempts+1,next_attempt=$2 WHERE id=(SELECT id FROM alert_outbox WHERE status='pending' AND next_attempt<=$1 ORDER BY next_attempt,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING event,status,attempts,next_attempt,last_error`, now, now.Add(lease)).Scan(&b, &d.Status, &d.Attempts, &d.NextAttempt, &d.LastError)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &d.Event); err != nil {
		return nil, err
	}
	return &d, nil
}
func (p *Postgres) Finish(ctx context.Context, id string, attempt int, next time.Time, message string) error {
	status := "pending"
	if message == "" {
		status = "sent"
	} else if attempt >= 8 {
		status = "failed"
	}
	_, err := p.pool.Exec(ctx, "UPDATE alert_outbox SET status=$3,next_attempt=$4,last_error=$5 WHERE id=$1 AND attempts=$2 AND status='pending'", id, attempt, status, next, message)
	return err
}
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }

// OperatorRoute reads only the durable deployment fence, not measurement rows.
func (p *Postgres) OperatorRoute(ctx context.Context, id string) (string, error) {
	var route string
	err := p.pool.QueryRow(ctx, "SELECT operator_route FROM slos WHERE id=$1", id).Scan(&route)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", model.ErrNotFound
	}
	return route, err
}
