// Command petri starts the proxy. Configuration comes from environment
// variables so it drops into a docker-compose stack with no flag wiring.
//
// Set PETRI_ADMIN_DSN to a superuser DSN and each client connection will
// land on its own forked database, named by a fresh UUID. Without
// PETRI_ADMIN_DSN, petri runs as a transparent proxy.
package main

import (
	"context"
	"errors"
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
	ListenAddr  string
	BackendAddr string
	AdminDSN    string // optional: enables per-connection forking when set
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		ListenAddr:  getenv("PETRI_LISTEN_ADDR"),
		BackendAddr: getenv("PETRI_BACKEND_ADDR"),
		AdminDSN:    getenv("PETRI_ADMIN_DSN"),
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":5432"
	}
	if cfg.BackendAddr == "" {
		return cfg, errors.New("PETRI_BACKEND_ADDR is required (host:port of the real Postgres)")
	}
	return cfg, nil
}

// run is main's testable core. Accepts an env-lookup function and a writer for
// startup logs so tests can drive it without touching real stdio.
func run(getenv func(string) string, logs io.Writer) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddr, err)
	}
	fmt.Fprintf(logs, "petri listening on %s, forwarding to %s (forking=%t)\n",
		ln.Addr(), cfg.BackendAddr, cfg.AdminDSN != "")

	p := &proxy.Proxy{
		BackendAddr: cfg.BackendAddr,
		OnStartup:   buildOnStartup(cfg, logs),
	}
	return p.Serve(ln)
}

// buildOnStartup picks the right hook based on config: a forking hook when
// PETRI_ADMIN_DSN is set, a logging-only hook otherwise.
func buildOnStartup(cfg config, logs io.Writer) func(*startup.Info) (func(), error) {
	if cfg.AdminDSN == "" {
		return logConnections(logs)
	}
	return forkPerConnection(&forker.Forker{AdminDSN: cfg.AdminDSN}, logs)
}

// forkerAPI is the slice of Forker we depend on, so tests can substitute a
// fake without spinning up a real Postgres.
type forkerAPI interface {
	Fork(ctx context.Context, templateName, forkName string) error
	Drop(ctx context.Context, forkName string) error
}

// forkPerConnection returns an OnStartup hook that gives every client its own
// freshly-forked database. The fork name and application_name are both
// rewritten to a UUID so the connection is fully isolated and traceable.
// The returned cleanup drops the fork when the client disconnects.
func forkPerConnection(f forkerAPI, logs io.Writer) func(*startup.Info) (func(), error) {
	return func(i *startup.Info) (func(), error) {
		template := i.Database
		forkName := newForkName()

		if err := withTimeout(func(ctx context.Context) error {
			return f.Fork(ctx, template, forkName)
		}); err != nil {
			fmt.Fprintf(logs, "fork failed (template=%q): %v\n", template, err)
			return nil, err
		}
		fmt.Fprintf(logs, "forked %q -> %q (client app=%q user=%q)\n",
			template, forkName, i.ApplicationName, i.User)

		i.Database = forkName
		i.ApplicationName = forkName

		cleanup := func() {
			if err := withTimeout(func(ctx context.Context) error {
				return f.Drop(ctx, forkName)
			}); err != nil {
				fmt.Fprintf(logs, "drop fork %q failed: %v\n", forkName, err)
				return
			}
			fmt.Fprintf(logs, "dropped fork %q\n", forkName)
		}
		return cleanup, nil
	}
}

// withTimeout runs fn against a fresh 30s context — short enough to bound
// admin operations, long enough for slow CREATE DATABASE on big templates.
func withTimeout(fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return fn(ctx)
}

// newForkName returns a Postgres-safe identifier built from a UUID.
// Dashes are stripped so the name doesn't need quoting in casual usage.
func newForkName() string {
	return "petri_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// logConnections is the Phase-2 hook: just print one line per client.
// No cleanup needed — there's nothing to undo.
func logConnections(logs io.Writer) func(*startup.Info) (func(), error) {
	return func(i *startup.Info) (func(), error) {
		fmt.Fprintf(logs, "client connected: app=%q db=%q user=%q\n",
			i.ApplicationName, i.Database, i.User)
		return nil, nil
	}
}
