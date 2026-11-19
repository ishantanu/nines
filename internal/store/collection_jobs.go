package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ishantanu/nines/internal/model"
)

var ErrCollectionLease = errors.New("collection lease expired or superseded")

type CollectionJob struct {
	ID                string
	Token             string
	Failures          int
	DueAt, LeaseUntil time.Time
}

// CollectionQueue is a bounded, coalescing queue: one durable row per SLO.
// Implementations own the clock; callers cannot prolong a lease by changing time.
type CollectionQueue interface {
	ClaimCollection(context.Context, []string, time.Duration) (*CollectionJob, error)
	FinishCollection(context.Context, CollectionJob, time.Duration, bool) error
}
type collectionLeaseKey struct{}

func WithCollectionLease(ctx context.Context, job CollectionJob) context.Context {
	return context.WithValue(ctx, collectionLeaseKey{}, job)
}

func (p *Postgres) ClaimCollection(ctx context.Context, ids []string, ttl time.Duration) (*CollectionJob, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if ttl <= 0 || ttl > 5*time.Minute {
		return nil, errors.New("invalid collection lease duration")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `INSERT INTO collection_jobs(slo_id) SELECT id FROM slos WHERE id=ANY($1::text[]) ON CONFLICT DO NOTHING`, ids); err != nil {
		return nil, err
	}
	var job CollectionJob
	err = tx.QueryRow(ctx, `WITH candidate AS (
 SELECT slo_id FROM collection_jobs WHERE slo_id=ANY($1::text[]) AND due_at<=clock_timestamp() AND lease_until<=clock_timestamp()
 ORDER BY due_at,slo_id FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE collection_jobs j SET token=$3, lease_until=clock_timestamp()+$2*interval '1 second'
 FROM candidate c WHERE j.slo_id=c.slo_id RETURNING j.slo_id,j.token,j.failures,j.due_at,j.lease_until`, ids, ttl.Seconds(), model.NewID()).Scan(&job.ID, &job.Token, &job.Failures, &job.DueAt, &job.LeaseUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &job, nil
}
func (p *Postgres) FinishCollection(ctx context.Context, job CollectionJob, delay time.Duration, failed bool) error {
	if delay < 0 || delay > 5*time.Minute {
		return errors.New("invalid collection retry delay")
	}
	failures := 0
	if failed {
		failures = min(job.Failures+1, 6)
	}
	tag, err := p.pool.Exec(ctx, `UPDATE collection_jobs SET due_at=clock_timestamp()+$3*interval '1 second',lease_until='epoch',failures=$4 WHERE slo_id=$1 AND token=$2 AND lease_until>clock_timestamp()`, job.ID, job.Token, delay.Seconds(), failures)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrCollectionLease
	}
	return nil
}
func checkCollectionLease(ctx context.Context, tx pgx.Tx, id string, lock bool) error {
	job, ok := ctx.Value(collectionLeaseKey{}).(CollectionJob)
	if !ok {
		return nil
	}
	if job.ID != id {
		return ErrCollectionLease
	}
	query := `SELECT token=$2 AND lease_until>clock_timestamp() FROM collection_jobs WHERE slo_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var valid bool
	err := tx.QueryRow(ctx, query, id, job.Token).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCollectionLease
	}
	if err != nil {
		return err
	}
	if !valid {
		return ErrCollectionLease
	}
	return nil
}

func (m *Memory) ClaimCollection(ctx context.Context, ids []string, ttl time.Duration) (*CollectionJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ttl <= 0 || ttl > 5*time.Minute {
		return nil, errors.New("invalid collection lease duration")
	}
	if m.collectionJobs == nil {
		m.collectionJobs = map[string]CollectionJob{}
	}
	now := time.Now()
	var chosen *CollectionJob
	for _, id := range ids {
		if _, ok := m.records[id]; !ok {
			continue
		}
		job, ok := m.collectionJobs[id]
		if !ok {
			job = CollectionJob{ID: id, DueAt: now}
			m.collectionJobs[id] = job
		}
		if job.DueAt.After(now) || job.LeaseUntil.After(now) {
			continue
		}
		if chosen == nil || job.DueAt.Before(chosen.DueAt) || job.DueAt.Equal(chosen.DueAt) && job.ID < chosen.ID {
			v := job
			chosen = &v
		}
	}
	if chosen != nil {
		chosen.Token = model.NewID()
		chosen.LeaseUntil = now.Add(ttl)
		m.collectionJobs[chosen.ID] = *chosen
	}
	return chosen, nil
}
func (m *Memory) FinishCollection(ctx context.Context, job CollectionJob, delay time.Duration, failed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay < 0 || delay > 5*time.Minute {
		return errors.New("invalid collection retry delay")
	}
	current, ok := m.collectionJobs[job.ID]
	if !ok || current.Token != job.Token || !current.LeaseUntil.After(time.Now()) {
		return ErrCollectionLease
	}
	current.DueAt = time.Now().Add(delay)
	current.LeaseUntil = time.Time{}
	current.Failures = 0
	if failed {
		current.Failures = min(job.Failures+1, 6)
	}
	m.collectionJobs[job.ID] = current
	return nil
}
func (m *Memory) checkCollectionLease(ctx context.Context, id string) error {
	job, ok := ctx.Value(collectionLeaseKey{}).(CollectionJob)
	if !ok {
		return nil
	}
	current, exists := m.collectionJobs[id]
	if job.ID != id || !exists || current.Token != job.Token || !current.LeaseUntil.After(time.Now()) {
		return ErrCollectionLease
	}
	return nil
}
