// Package forker creates per-test Postgres databases by cloning a template.
//
// A Fork is a fresh database whose contents are bit-identical to its template
// at the moment of CREATE — Postgres copies the on-disk pages directly via
// CREATE DATABASE … TEMPLATE …. This makes seeded databases nearly free to
// duplicate and isolates each client's writes from the rest.
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

// Forker creates and drops fork databases using a single admin connection
// string. The AdminDSN must authenticate as a role that can CREATE DATABASE
// (typically a superuser) and point at any database other than the template.
type Forker struct {
	AdminDSN string
}

// Fork creates a new database `forkName` whose contents copy `templateName`.
// The template must have no other open connections at the moment of fork.
func (f *Forker) Fork(ctx context.Context, templateName, forkName string) error {
	conn, err := pgx.Connect(ctx, f.AdminDSN)
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

// Drop deletes a fork database. Used for cleanup; idempotent on
// already-gone databases via IF EXISTS.
func (f *Forker) Drop(ctx context.Context, forkName string) error {
	conn, err := pgx.Connect(ctx, f.AdminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close(ctx)

	stmt := fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(forkName))
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("drop database %q: %w", forkName, err)
	}
	return nil
}

// quoteIdent wraps an identifier in double quotes and escapes embedded quotes,
// matching Postgres rules for delimited identifiers. We never pass identifiers
// through query parameters because DDL doesn't support them.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
