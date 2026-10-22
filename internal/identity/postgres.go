package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/internal/observability"
)

// Records have independent indexed keys; session reads do not load a directory.
// Transactions use a tenant-specific advisory lock for membership invariants.
// JSON payloads are typed in the service; keys and routing are never caller SQL.
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	base := cfg.ConnConfig.RuntimeParams["search_path"]
	if base == "" {
		base = "public"
	}
	sum := sha256.Sum256([]byte(base))
	schema := "waypoint_identity_" + hex.EncodeToString(sum[:6])
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 5
	cfg.ConnConfig.Tracer = observability.PGTracer{}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Postgres, error) { p.Close(); return nil, err }
	tx, err := p.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", schema); err != nil {
		return fail(err)
	}
	if _, err = tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return fail(err)
	}
	_, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS identity_metadata(key text PRIMARY KEY,value text NOT NULL);
 CREATE TABLE IF NOT EXISTS identity_records(kind text NOT NULL,scope text NOT NULL,id text NOT NULL,data jsonb NOT NULL,expires_at timestamptz,PRIMARY KEY(kind,scope,id));
 CREATE INDEX IF NOT EXISTS identity_expiration ON identity_records(expires_at) WHERE expires_at IS NOT NULL;
 INSERT INTO identity_metadata(key,value) VALUES('schema_version','1') ON CONFLICT DO NOTHING;`)
	if err != nil {
		return fail(err)
	}
	var version string
	if err = tx.QueryRow(ctx, "SELECT value FROM identity_metadata WHERE key='schema_version'").Scan(&version); err != nil {
		return fail(err)
	}
	if version != "1" {
		return fail(errors.New("unsupported identity schema version"))
	}
	if err = tx.Commit(ctx); err != nil {
		return fail(err)
	}
	return &Postgres{p}, nil
}
func (p *Postgres) Close() { p.pool.Close() }
func (p *Postgres) View(ctx context.Context, f func(Tx) error) error {
	return p.transaction(ctx, "", false, f)
}
func (p *Postgres) Update(ctx context.Context, lock string, f func(Tx) error) error {
	return p.transaction(ctx, "identity-write", true, f)
}
func (p *Postgres) transaction(ctx context.Context, lock string, write bool, f func(Tx) error) error {
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if write {
		opts = pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	}
	tx, err := p.pool.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()
	if write {
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(current_schema() || ':' || $1,0))", lock); err != nil {
			return err
		}
	}
	if err = f(&postgresTx{ctx, tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Prune(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM identity_records WHERE expires_at < now()")
	return err
}

type postgresTx struct {
	ctx context.Context
	tx  pgx.Tx
}

func (t *postgresTx) Get(kind, scope, id string) (json.RawMessage, error) {
	var b []byte
	err := t.tx.QueryRow(t.ctx, "SELECT data FROM identity_records WHERE kind=$1 AND scope=$2 AND id=$3 AND (expires_at IS NULL OR expires_at>now())", kind, scope, id).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return b, err
}
func (t *postgresTx) List(kind, scope string) ([]json.RawMessage, error) {
	rows, err := t.tx.Query(t.ctx, "SELECT data FROM identity_records WHERE kind=$1 AND scope=$2 AND (expires_at IS NULL OR expires_at>now()) ORDER BY id LIMIT $3", kind, scope, maxRows+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if len(out) > maxRows {
		return nil, errors.New("identity scope exceeds 1000 records")
	}
	return out, rows.Err()
}
func (t *postgresTx) Put(kind, scope, id string, b json.RawMessage, expires time.Time) error {
	var expiration any
	if !expires.IsZero() {
		expiration = expires
	}
	_, err := t.tx.Exec(t.ctx, "INSERT INTO identity_records(kind,scope,id,data,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(kind,scope,id) DO UPDATE SET data=excluded.data,expires_at=excluded.expires_at", kind, scope, id, []byte(b), expiration)
	return err
}
func (t *postgresTx) Delete(kind, scope, id string) error {
	_, err := t.tx.Exec(t.ctx, "DELETE FROM identity_records WHERE kind=$1 AND scope=$2 AND id=$3", kind, scope, id)
	return err
}

func (t *postgresTx) Page(kind, scope, before string, limit int) ([]json.RawMessage, error) {
	rows, err := t.tx.Query(t.ctx, "SELECT data FROM identity_records WHERE kind=$1 AND scope=$2 AND ($3='' OR id<$3) AND (expires_at IS NULL OR expires_at>now()) ORDER BY id DESC LIMIT $4", kind, scope, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (t *postgresTx) Ensure(kind, scope, id string, b json.RawMessage) error {
	_, err := t.tx.Exec(t.ctx, "INSERT INTO identity_records(kind,scope,id,data) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", kind, scope, id, []byte(b))
	return err
}
