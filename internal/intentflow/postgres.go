package intentflow

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/observability"
)

type postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (Repository, error) {
	p, e := observability.NewPool(ctx, dsn)
	if e != nil {
		return nil, e
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(88621950)`); e == nil {
		_, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS intent_conversations(id text PRIMARY KEY,summary jsonb NOT NULL,document bytea NOT NULL,status text NOT NULL,lease_until timestamptz NOT NULL);CREATE INDEX IF NOT EXISTS intent_conversations_queue ON intent_conversations(status,lease_until,id)`)
	}
	journalExisted := true
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT to_regclass('intent_audit_events') IS NOT NULL`).Scan(&journalExisted)
	}
	if e == nil {
		_, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS intent_audit_events(id uuid PRIMARY KEY,tenant text NOT NULL,conversation text NOT NULL,sequence bigint NOT NULL,at timestamptz NOT NULL,actor text NOT NULL,action text NOT NULL,outcome text NOT NULL,resources text[] NOT NULL,document bytea NOT NULL,summary bytea,UNIQUE(tenant,conversation,sequence)); CREATE INDEX IF NOT EXISTS intent_audit_time ON intent_audit_events(tenant,at DESC,id DESC);ALTER TABLE intent_audit_events ADD COLUMN IF NOT EXISTS summary bytea`)
	}
	if e == nil && !journalExisted {
		e = bootstrapAuditJournal(ctx, tx)
	}
	if e == nil {
		_, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS intent_audit_archives(tenant text NOT NULL,target text NOT NULL,record_id uuid NOT NULL,token text NOT NULL,lease_until timestamptz NOT NULL,next_attempt timestamptz NOT NULL,retain_until timestamptz NOT NULL,attempts integer NOT NULL,last_error text NOT NULL DEFAULT '',receipt jsonb,PRIMARY KEY(tenant,target,record_id));CREATE INDEX IF NOT EXISTS intent_audit_archive_queue ON intent_audit_archives(tenant,target,next_attempt,lease_until) WHERE receipt IS NULL`)
	}
	if e == nil {
		_, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS intent_webhook_deliveries(tenant text NOT NULL,target text NOT NULL,record_id uuid NOT NULL,token text NOT NULL,lease_until timestamptz NOT NULL,next_attempt timestamptz NOT NULL,attempts integer NOT NULL,last_error text NOT NULL DEFAULT '',delivered boolean NOT NULL DEFAULT false,PRIMARY KEY(tenant,target,record_id));CREATE INDEX IF NOT EXISTS intent_webhook_queue ON intent_webhook_deliveries(tenant,target,next_attempt,lease_until) WHERE NOT delivered`)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		_ = tx.Rollback(context.Background())
		p.Close()
		return nil, e
	}
	return &postgres{p}, nil
}
func (p *postgres) Close() { p.pool.Close() }
func decodeRow(row pgx.Row) (record, error) {
	var b []byte
	var r record
	e := row.Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func (p *postgres) Read(ctx context.Context, id string) (record, error) {
	return decodeRow(p.pool.QueryRow(ctx, `SELECT document FROM intent_conversations WHERE id=$1`, id))
}
func writeRecord(ctx context.Context, tx pgx.Tx, r record) error {
	if e := validSize(r); e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	s, _ := json.Marshal(r.Session.IntentConversationSummary)
	_, e = tx.Exec(ctx, `UPDATE intent_conversations SET summary=$2,document=$3,status=$4,lease_until=$5 WHERE id=$1`, r.Session.ID, s, b, r.Session.State, r.LeaseUntil)
	return e
}
func (p *postgres) Create(ctx context.Context, r record) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(88621951)`); e != nil {
		return record{}, e
	}
	old, e := decodeRow(tx.QueryRow(ctx, `SELECT document FROM intent_conversations WHERE id=$1`, r.Session.ID))
	if e == nil {
		if old.RequestDigest != r.RequestDigest {
			return record{}, ErrConflict
		}
		return old, tx.Commit(ctx)
	}
	if e != ErrNotFound {
		return record{}, e
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM intent_conversations`).Scan(&n); e != nil {
		return record{}, e
	}
	if n >= maxSessions {
		return record{}, ErrLimit
	}
	if e = validSize(r); e != nil {
		return record{}, e
	}
	b, _ := json.Marshal(r)
	s, _ := json.Marshal(r.Session.IntentConversationSummary)
	if _, e = tx.Exec(ctx, `INSERT INTO intent_conversations VALUES($1,$2,$3,$4,$5)`, r.Session.ID, s, b, r.Session.State, r.LeaseUntil); e != nil {
		return record{}, e
	}
	return clone(r), tx.Commit(ctx)
}
func (p *postgres) List(ctx context.Context, after string, limit int) ([]v1.IntentConversationSummary, error) {
	rows, e := p.pool.Query(ctx, `SELECT summary FROM intent_conversations WHERE id>$1 ORDER BY id LIMIT $2`, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []v1.IntentConversationSummary{}
	for rows.Next() {
		var b []byte
		var s v1.IntentConversationSummary
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &s); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (p *postgres) Update(ctx context.Context, id string, fn func(*record, time.Time) error) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	var now time.Time
	e = tx.QueryRow(ctx, `SELECT document,clock_timestamp() FROM intent_conversations WHERE id=$1 FOR UPDATE`, id).Scan(&b, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return record{}, ErrNotFound
	}
	if e != nil {
		return record{}, e
	}
	var r record
	if e = json.Unmarshal(b, &r); e != nil {
		return record{}, e
	}
	old := clone(r)
	sequence, _ := lastAudit(old.Session.Audit)
	if sequence > 0 {
		var raw []byte
		var head v1.AuditRecord
		err := tx.QueryRow(ctx, `SELECT COALESCE(summary,document) FROM intent_audit_events WHERE tenant=$1 AND conversation=$2 AND sequence=$3`, old.Session.Tenant, id, sequence).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return record{}, validJournalHead(old, nil)
		}
		if err != nil {
			return record{}, err
		}
		if err = json.Unmarshal(raw, &head); err != nil {
			return record{}, err
		}
		if err = validJournalHead(old, &head); err != nil {
			return record{}, err
		}
	}
	if e = fn(&r, now.UTC()); e != nil {
		return record{}, e
	}
	added, err := journalAppend(old, r)
	if err != nil {
		return record{}, err
	}
	if err = insertAuditRecords(ctx, tx, added); err != nil {
		return record{}, err
	}
	if e = writeRecord(ctx, tx, r); e != nil {
		return record{}, e
	}
	return r, tx.Commit(ctx)
}
func (p *postgres) Audit(ctx context.Context, search AuditSearch) ([]v1.AuditRecord, error) {
	q := search.Query
	order := "at DESC,id DESC"
	if search.Ascending {
		order = "sequence ASC"
	}
	var from, to, before any
	if !q.From.IsZero() {
		from = q.From
	}
	if !q.To.IsZero() {
		to = q.To
	}
	if !search.BeforeAt.IsZero() {
		before = search.BeforeAt
	}
	beforeID := search.BeforeID
	if beforeID == "" {
		beforeID = "00000000-0000-0000-0000-000000000000"
	}
	var rows pgx.Rows
	var err error
	if search.ID != "" {
		rows, err = p.pool.Query(ctx, `SELECT document FROM intent_audit_events WHERE tenant=$1 AND id=$2::uuid`, search.Tenant, search.ID)
	} else {
		rows, err = p.pool.Query(ctx, `SELECT COALESCE(summary,convert_to((convert_from(document,'UTF8')::jsonb-'proposal')::text,'UTF8')) FROM intent_audit_events WHERE tenant=$1 AND ($2='' OR conversation=$2) AND ($3='' OR actor=$3) AND ($4='' OR action=$4) AND ($5='' OR outcome=$5) AND ($6='' OR $6=ANY(resources)) AND ($7::timestamptz IS NULL OR at>=$7) AND ($8::timestamptz IS NULL OR at<=$8) AND ($9::timestamptz IS NULL OR (at,id)<($9,$10::uuid)) AND sequence>$11 AND ($13::bigint=0 OR sequence<=$13) ORDER BY `+order+` LIMIT $12`, search.Tenant, q.Conversation, q.Actor, q.Action, q.Outcome, q.Resource, from, to, before, beforeID, search.AfterSequence, q.Limit, search.ThroughSequence)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []v1.AuditRecord{}
	for rows.Next() {
		var raw []byte
		var event v1.AuditRecord
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
func (p *postgres) Claim(ctx context.Context, token string) (record, error) {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return record{}, e
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	var b []byte
	var now time.Time
	e = tx.QueryRow(ctx, `SELECT document,clock_timestamp() FROM intent_conversations WHERE status IN ('queued','running') AND lease_until<=clock_timestamp() ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&b, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return record{}, ErrNotFound
	}
	if e != nil {
		return record{}, e
	}
	var r record
	if e = json.Unmarshal(b, &r); e != nil {
		return record{}, e
	}
	if e = claim(&r, token, now.UTC()); e != nil {
		return record{}, e
	}
	if e = writeRecord(ctx, tx, r); e != nil {
		return record{}, e
	}
	return r, tx.Commit(ctx)
}

func insertAuditRecords(ctx context.Context, tx pgx.Tx, added []v1.AuditRecord) error {
	for _, event := range added {
		document, err := json.Marshal(event)
		if err != nil {
			return err
		}
		brief := event
		brief.Proposal = nil
		summary, err := json.Marshal(brief)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO intent_audit_events(id,tenant,conversation,sequence,at,actor,action,outcome,resources,document,summary) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, event.ID, event.Tenant, event.Conversation, event.Event.Sequence, event.Event.At, event.Event.Actor.Subject, event.Event.Action, event.Event.Outcome, auditResources(event), document, summary); err != nil {
			return err
		}
	}
	return nil
}

// Bootstrap only when introducing the journal, so ordinary startup cannot conceal
// deleted journal rows. Retained snapshots are explicitly marked as reconstructed.
func bootstrapAuditJournal(ctx context.Context, tx pgx.Tx) error {
	after := ""
	for {
		rows, err := tx.Query(ctx, `SELECT document FROM intent_conversations WHERE id>$1 ORDER BY id LIMIT 20`, after)
		if err != nil {
			return err
		}
		batch := []record{}
		for rows.Next() {
			var raw []byte
			var item record
			if err = rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err = json.Unmarshal(raw, &item); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for _, item := range batch {
			after = item.Session.ID
			events := item.Session.Audit
			if len(events) == 0 || events[0].Sequence != 1 || !auditChainValid(events) || !frozenReviewsValid(item.Session) {
				continue
			}
			added, err := journalAppend(record{}, item)
			if err != nil {
				return err
			}
			for i := range added {
				added[i].RecoveredFromConversation = true
			}
			if err = insertAuditRecords(ctx, tx, added); err != nil {
				return err
			}
		}
	}
}
