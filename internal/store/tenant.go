package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/ishantanu/nines/internal/observability"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ishantanu/nines/internal/tenancy"
)

// TenantDatabaseURL creates a fixed schema and returns a DSN whose search_path
// contains only that tenant's schema. A missing table cannot fall through to
// another tenant's data. 'default' preserves the legacy configured schema.
func TenantDatabaseURL(ctx context.Context, base, id string) (string, error) {
	if !tenancy.ValidID(id) {
		return "", fmt.Errorf("invalid tenant ID")
	}
	if id == "default" {
		return base, nil
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", fmt.Errorf("tenant databases require a postgres:// URL")
	}
	config, err := pgxpool.ParseConfig(base)
	if err != nil {
		return "", err
	}
	// Include the configured base namespace so test/deployment namespaces do not
	// alias one another when they happen to share a database.
	namespace := config.ConnConfig.RuntimeParams["search_path"]
	if namespace == "" {
		namespace = "public"
	}
	sum := sha256.Sum256([]byte(namespace + "\x00" + id))
	schemaName := "wp_tenant_" + hex.EncodeToString(sum[:16])
	config.ConnConfig.Tracer = observability.PGTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return "", err
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schemaName}.Sanitize()); err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("search_path", schemaName)
	u.RawQuery = query.Encode()
	return strings.TrimSpace(u.String()), nil
}
