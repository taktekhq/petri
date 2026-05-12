// Command petri is the Postgres proxy that gives each client connection its
// own forked database. Configuration follows the upstream postgres image —
// POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, PGPORT — so the bundled
// petri:postgres image is a drop-in replacement.
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

// forkerAPI is the slice of Forker we depend on, so tests can substitute a fake.
type forkerAPI interface {
	Fork(ctx context.Context, adminDSN, templateName, forkName string) error
	Drop(ctx context.Context, adminDSN, forkName string) error
}

const adminTimeout = 30 * time.Second // bounds Fork/Drop; allows slow CREATE DATABASE on big templates

// forkPerConnection returns an OnStartup hook that forks the client's
// requested database into a UUID-named copy. The admin connection always uses
// POSTGRES_USER/PASSWORD so forking works even when the client is read-only;
// the client's own queries bridge through with their own credentials.
func forkPerConnection(cfg config, f forkerAPI, logs io.Writer) func(*startup.Info) (func(), error) {
	adminDSN := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%s/postgres?sslmode=disable",
		cfg.AdminUser, cfg.AdminPassword, cfg.BackendPort)

	return func(i *startup.Info) (func(), error) {
		template := i.Database
		forkName := "petri_" + strings.ReplaceAll(uuid.NewString(), "-", "")

		ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
		defer cancel()
		if err := f.Fork(ctx, adminDSN, template, forkName); err != nil {
			fmt.Fprintf(logs, "fork failed (template=%q user=%q): %v\n", template, i.User, err)
			return nil, err
		}
		fmt.Fprintf(logs, "forked %q -> %q (user=%q app=%q)\n",
			template, forkName, i.User, i.ApplicationName)

		i.Database = forkName
		i.ApplicationName = forkName

		cleanup := func() {
			ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
			defer cancel()
			if err := f.Drop(ctx, adminDSN, forkName); err != nil {
				fmt.Fprintf(logs, "drop fork %q failed: %v\n", forkName, err)
				return
			}
			fmt.Fprintf(logs, "dropped fork %q\n", forkName)
		}
		return cleanup, nil
	}
}
