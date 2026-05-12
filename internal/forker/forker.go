// Package forker creates per-test Postgres databases by cloning a template
// via CREATE DATABASE … TEMPLATE … — a file-level copy, fast for seeded DBs.
//
// Forker is stateless; each call takes its own admin DSN.
package forker

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Forker creates and drops fork databases. The receiver is empty so callers
// can substitute a fake via an interface.
type Forker struct{}

// Fork creates a new database forkName whose contents copy templateName. The
// template must have no other open connections at the moment of fork.
func (Forker) Fork(ctx context.Context, adminDSN, templateName, forkName string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close(ctx)

	stmt := fmt.Sprintf(`CREATE DATABASE %s TEMPLATE %s`,
		quoteIdent(forkName), quoteIdent(templateName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create database %q: %w", forkName, err)
	}
	return nil
}

// Drop deletes a fork database (idempotent via IF EXISTS). Straggler sessions
// on the fork are evicted first — without this, the proxy's just-closed
// backend session can race with DROP and produce "database is being accessed".
func (Forker) Drop(ctx context.Context, adminDSN, forkName string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid != pg_backend_pid()`, forkName); err != nil {
		return fmt.Errorf("terminate connections to %q: %w", forkName, err)
	}

	stmt := fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(forkName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("drop database %q: %w", forkName, err)
	}
	return nil
}

// quoteIdent wraps an identifier in double quotes and escapes embedded ones.
// DDL doesn't support query parameters, so identifiers go through this.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
