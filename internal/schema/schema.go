// Package schema applies ordered, recorded schema migrations to PostgreSQL.
//
// Each component that owns tables (the SLO store, identity, workflows and so
// on) declares its migrations as a numbered list. Applied migrations are
// recorded in nines_schema_migrations, one row per component and migration,
// so a database always says which schema it has. Migrations only move
// forward: rolling back a release means restoring the backup taken before the
// upgrade, never running an older binary against a newer schema.
//
// Rules for changing a schema:
//   - Never edit or remove a migration that has been released. Add a new one
//     with the next ID. The migration checksum test enforces this.
//   - A migration runs once, inside the caller's transaction, which must hold
//     the component's advisory lock so concurrent servers apply it once.
//   - Migrations should keep the previous release able to read the data only
//     when that is cheap; the supported path is upgrade forward or restore.
package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Migration is one forward schema change. Set exactly one of SQL or Run.
type Migration struct {
	ID   int
	Name string
	// SQL is executed as a single statement batch.
	SQL string
	// Run is for changes that need logic, such as backfills. Its Version is
	// part of the checksum, so changing its behavior requires a new migration.
	Run     func(context.Context, pgx.Tx) error
	Version string
}

// Checksum identifies a migration's content for the released-migration test.
func (m Migration) Checksum() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s", m.ID, m.Name, m.SQL, m.Version)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ErrNewerSchema means the database was migrated by a newer Nines release.
var ErrNewerSchema = errors.New("database schema is newer than this Nines release")

const ledger = `CREATE TABLE IF NOT EXISTS nines_schema_migrations(
 component text NOT NULL,
 id integer NOT NULL,
 name text NOT NULL,
 checksum text NOT NULL,
 applied_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(component, id))`

// Validate checks that migrations are numbered 1..n with names and exactly one
// body each.
func Validate(component string, migrations []Migration) error {
	if component == "" {
		return fmt.Errorf("migrations need a component name")
	}
	for i, m := range migrations {
		if m.ID != i+1 {
			return fmt.Errorf("%s migration %q has ID %d, want %d: IDs must be consecutive from 1", component, m.Name, m.ID, i+1)
		}
		if m.Name == "" || (m.SQL == "") == (m.Run == nil) {
			return fmt.Errorf("%s migration %d needs a name and exactly one of SQL or Run", component, m.ID)
		}
		if m.Run != nil && m.Version == "" {
			return fmt.Errorf("%s migration %d runs code and needs a Version", component, m.ID)
		}
	}
	return nil
}

// Apply runs the component's pending migrations in order within tx and records
// each one. It refuses a database that already has migrations this binary does
// not know, because that database was upgraded by a newer release.
func Apply(ctx context.Context, tx pgx.Tx, component string, migrations []Migration) error {
	if err := Validate(component, migrations); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, ledger); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	var latest int
	var latestName string
	err := tx.QueryRow(ctx, `SELECT id, name FROM nines_schema_migrations WHERE component=$1 ORDER BY id DESC LIMIT 1`, component).Scan(&latest, &latestName)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read %s migrations: %w", component, err)
	}
	if latest > len(migrations) {
		return fmt.Errorf("%w: %s is at migration %d (%s), but this release knows only %d. Run the release that upgraded the database, or restore the backup taken before that upgrade", ErrNewerSchema, component, latest, latestName, len(migrations))
	}
	for _, m := range migrations[latest:] {
		if m.Run != nil {
			err = m.Run(ctx, tx)
		} else {
			_, err = tx.Exec(ctx, m.SQL)
		}
		if err != nil {
			return fmt.Errorf("%s migration %d (%s): %w", component, m.ID, m.Name, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO nines_schema_migrations(component, id, name, checksum) VALUES($1,$2,$3,$4)`, component, m.ID, m.Name, m.Checksum()); err != nil {
			return fmt.Errorf("record %s migration %d: %w", component, m.ID, err)
		}
	}
	return nil
}
