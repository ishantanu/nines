package schema

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidate(t *testing.T) {
	ok := []Migration{{ID: 1, Name: "a", SQL: "SELECT 1"}, {ID: 2, Name: "b", Run: func(context.Context, pgx.Tx) error { return nil }, Version: "v"}}
	if err := Validate("c", ok); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]Migration{
		"gap":       {{ID: 1, Name: "a", SQL: "x"}, {ID: 3, Name: "b", SQL: "y"}},
		"zero":      {{ID: 0, Name: "a", SQL: "x"}},
		"no name":   {{ID: 1, SQL: "x"}},
		"both":      {{ID: 1, Name: "a", SQL: "x", Run: func(context.Context, pgx.Tx) error { return nil }, Version: "v"}},
		"neither":   {{ID: 1, Name: "a"}},
		"unversion": {{ID: 1, Name: "a", Run: func(context.Context, pgx.Tx) error { return nil }}},
	} {
		if Validate("c", bad) == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestChecksumCoversContent(t *testing.T) {
	a := Migration{ID: 1, Name: "a", SQL: "CREATE TABLE t(x int)"}
	b := a
	b.SQL = "CREATE TABLE t(x bigint)"
	if a.Checksum() == b.Checksum() || a.Checksum() != (Migration{ID: 1, Name: "a", SQL: "CREATE TABLE t(x int)"}).Checksum() {
		t.Fatal("checksum does not follow migration content")
	}
}

// database returns a pool on a fresh schema of TEST_DATABASE_URL.
func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	name := "migrate_" + strings.ReplaceAll(t.Name(), "/", "_")
	name = strings.ToLower(name)
	if _, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+name+" CASCADE; CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+name+" CASCADE")
		admin.Close()
	})
	return pool
}

func apply(t *testing.T, pool *pgxpool.Pool, ms []Migration) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = Apply(ctx, tx, "component", ms); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestApplyRecordsAndRunsOnce(t *testing.T) {
	pool := database(t)
	runs := 0
	ms := []Migration{
		{ID: 1, Name: "create", SQL: "CREATE TABLE things(id int PRIMARY KEY)"},
		{ID: 2, Name: "backfill", Version: "1", Run: func(ctx context.Context, tx pgx.Tx) error {
			runs++
			_, err := tx.Exec(ctx, "INSERT INTO things VALUES (1)")
			return err
		}},
	}
	for i := 0; i < 2; i++ {
		if err := apply(t, pool, ms); err != nil {
			t.Fatal(err)
		}
	}
	if runs != 1 {
		t.Fatalf("migration ran %d times, want once", runs)
	}
	var recorded int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM nines_schema_migrations WHERE component='component'").Scan(&recorded); err != nil || recorded != 2 {
		t.Fatalf("ledger has %d rows: %v", recorded, err)
	}
	// A later release appends a migration; only that one runs.
	ms = append(ms, Migration{ID: 3, Name: "column", SQL: "ALTER TABLE things ADD COLUMN note text"})
	if err := apply(t, pool, ms); err != nil || runs != 1 {
		t.Fatalf("append: %v, runs %d", err, runs)
	}
}

func TestNewerDatabaseIsRefused(t *testing.T) {
	pool := database(t)
	newer := []Migration{{ID: 1, Name: "create", SQL: "CREATE TABLE things(id int)"}, {ID: 2, Name: "add note", SQL: "ALTER TABLE things ADD COLUMN note text"}}
	if err := apply(t, pool, newer); err != nil {
		t.Fatal(err)
	}
	err := apply(t, pool, newer[:1])
	if !errors.Is(err, ErrNewerSchema) || !strings.Contains(err.Error(), "migration 2 (add note)") || !strings.Contains(err.Error(), "restore the backup") {
		t.Fatalf("an older release accepted a newer database: %v", err)
	}
}

func TestFailedMigrationLeavesNothingBehind(t *testing.T) {
	pool := database(t)
	ms := []Migration{{ID: 1, Name: "create", SQL: "CREATE TABLE things(id int)"}, {ID: 2, Name: "broken", SQL: "ALTER TABLE missing ADD COLUMN x int"}}
	if err := apply(t, pool, ms); err == nil || !strings.Contains(err.Error(), "migration 2 (broken)") {
		t.Fatalf("broken migration: %v", err)
	}
	var exists bool
	if err := pool.QueryRow(context.Background(), "SELECT to_regclass('things') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("a failed upgrade left a partial schema", err)
	}
}
