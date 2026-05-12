// Command petri is the Postgres proxy that gives each client connection its
// own isolated forked database.
//
// Configuration follows the upstream postgres docker image: POSTGRES_PASSWORD,
// POSTGRES_USER, POSTGRES_DB, PGPORT — no petri-specific env vars to learn.
// The bundled petri:postgres image runs Postgres on the loopback PGPORT and
// petri on :5432, so a docker-compose stack only needs `image: petri:postgres`
// in place of the usual `image: postgres:16`.
//
// For every client connection, petri:
//  1. reads the client's startup message (user, database)
//  2. opens an admin connection using POSTGRES_USER + POSTGRES_PASSWORD —
//     petri's own DB work (Fork / Drop) always runs as the admin role so it
//     works even when the client is connecting as a read-only user
//  3. forks <database> into a UUID-named copy
//  4. rewrites the startup to point at the fork and bridges the rest
//  5. drops the fork when the client disconnects
//
// The client's own queries are bridged through with their own credentials, so
// permission boundaries (read-only roles, row-level security, etc.) are
// preserved end-to-end.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/taktekhq/petri/internal/forker"
	"github.com/taktekhq/petri/internal/proxy"
	"github.com/taktekhq/petri/internal/startup"
)

func main() {
	if err := run(os.Getenv, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "petri:", err)
		os.Exit(1)
	}
}

type config struct {
	ListenAddr    string // PETRI_LISTEN_ADDR; default ":5432"
	BackendPort   string // PGPORT; default "5432" (bundled image sets 5433)
	AdminUser     string // POSTGRES_USER; default "postgres"
	AdminPassword string // POSTGRES_PASSWORD
}

func loadConfig(getenv func(string) string) config {
	return config{
		ListenAddr:    envOr(getenv, "PETRI_LISTEN_ADDR", ":5432"),
		BackendPort:   envOr(getenv, "PGPORT", "5432"),
		AdminUser:     envOr(getenv, "POSTGRES_USER", "postgres"),
		AdminPassword: getenv("POSTGRES_PASSWORD"),
	}
}

func envOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

// run is main's testable core: parse config, bind the listener, serve the proxy.
func run(getenv func(string) string, logs io.Writer) error {
	cfg := loadConfig(getenv)

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddr, err)
	}
	fmt.Fprintf(logs, "petri listening on %s, forwarding to 127.0.0.1:%s\n",
		ln.Addr(), cfg.BackendPort)

	p := &proxy.Proxy{
		BackendAddr: "127.0.0.1:" + cfg.BackendPort,
		OnStartup:   forkPerConnection(cfg, forker.Forker{}, logs),
	}
	return p.Serve(ln)
}

// forkerAPI is the slice of Forker we depend on, so tests can substitute a
// fake without spinning up a real Postgres.
type forkerAPI interface {
	Fork(ctx context.Context, adminDSN, templateName, forkName string) error
	Drop(ctx context.Context, adminDSN, forkName string) error
}

// forkPerConnection returns an OnStartup hook that gives every client its own
// freshly-forked database. The fork's database name and application_name are
// both rewritten to a UUID so the connection is fully isolated and traceable.
// Petri's own DB work (Fork / Drop) always uses POSTGRES_USER + POSTGRES_PASSWORD
// so it succeeds even when the client is connecting as a read-only role.
// The client's own queries are bridged through with their credentials, so
// permission boundaries (read-only roles, RLS, etc.) are preserved.
func forkPerConnection(cfg config, f forkerAPI, logs io.Writer) func(*startup.Info) (func(), error) {
	adminDSN := buildAdminDSN(cfg.AdminUser, cfg.AdminPassword, cfg.BackendPort)
	return func(i *startup.Info) (func(), error) {
		template := i.Database
		forkName := newForkName()

		if err := withTimeout(func(ctx context.Context) error {
			return f.Fork(ctx, adminDSN, template, forkName)
		}); err != nil {
			fmt.Fprintf(logs, "fork failed (template=%q user=%q): %v\n", template, i.User, err)
			return nil, err
		}
		fmt.Fprintf(logs, "forked %q -> %q (user=%q app=%q)\n",
			template, forkName, i.User, i.ApplicationName)

		i.Database = forkName
		i.ApplicationName = forkName

		cleanup := func() {
			if err := withTimeout(func(ctx context.Context) error {
				return f.Drop(ctx, adminDSN, forkName)
			}); err != nil {
				fmt.Fprintf(logs, "drop fork %q failed: %v\n", forkName, err)
				return
			}
			fmt.Fprintf(logs, "dropped fork %q\n", forkName)
		}
		return cleanup, nil
	}
}

// buildAdminDSN composes the per-connection admin DSN. Host is always loopback
// since petri is intended to run as a sidecar to Postgres (and inside the
// bundled image, on the same process tree).
func buildAdminDSN(user, password, port string) string {
	return fmt.Sprintf("postgres://%s:%s@127.0.0.1:%s/postgres?sslmode=disable",
		user, password, port)
}

// newForkName returns a Postgres-safe identifier built from a UUID. Dashes are
// stripped so the name doesn't need quoting in casual usage.
func newForkName() string {
	return "petri_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// withTimeout runs fn against a fresh 30s context — short enough to bound
// admin operations, long enough for slow CREATE DATABASE on big templates.
func withTimeout(fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return fn(ctx)
}
