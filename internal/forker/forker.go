// Package forker creates per-test Postgres databases by cloning a template.
//
// A Fork is a fresh database whose contents are bit-identical to its template
// at the moment of CREATE — Postgres copies the on-disk pages directly via
// CREATE DATABASE … TEMPLATE …. This makes seeded databases nearly free to
// duplicate and isolates each client's writes from the rest.
//
// Forker is stateless. Each call takes an admin DSN so callers can authenticate
// as a per-connection user (typically the client's username from the proxied
// startup message). That way auth and authorization failures surface as the
// client's own error, not as a side effect of admin credentials they never
// sent.
//
// File map:
//   - forker.go       – Forker.Fork, Forker.Drop, identifier quoting
//   - forker_test.go  – real Postgres in a testcontainer
package forker

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Forker creates and drops fork databases. The receiver is empty — it exists
// so callers can substitute a fake via an interface.
type Forker struct{}

// Fork creates a new database `forkName` whose contents copy `templateName`,
// connecting via adminDSN. The template must have no other open connections
// at the moment of fork.
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

// Drop deletes a fork database, connecting via adminDSN. Idempotent on
// already-gone databases via IF EXISTS. Stale connections to the fork
// (e.g., the proxy's just-closed backend session) are terminated first so
// DROP doesn't lose to a race with Postgres's own cleanup.
func (Forker) Drop(ctx context.Context, adminDSN, forkName string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close(ctx)

	if err := terminateConnectionsTo(ctx, conn, forkName); err != nil {
		return err
	}

	stmt := fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(forkName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("drop database %q: %w", forkName, err)
	}
	return nil
}

// terminateConnectionsTo evicts every backend currently attached to dbName
// other than our own. Best-effort: a failure here is reported but we still
// try the DROP — typically nothing was attached anyway.
func terminateConnectionsTo(ctx context.Context, conn *pgx.Conn, dbName string) error {
	_, err := conn.Exec(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid != pg_backend_pid()`, dbName)
	if err != nil {
		return fmt.Errorf("terminate connections to %q: %w", dbName, err)
	}
	return nil
}

// quoteIdent wraps an identifier in double quotes and escapes embedded quotes,
// matching Postgres rules for delimited identifiers. We never pass identifiers
// through query parameters because DDL doesn't support them.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
