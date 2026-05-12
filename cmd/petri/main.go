// Command petri is the Postgres proxy that fronts a single backend Postgres
// with two listeners: a passthrough port that proxies connections verbatim,
// and a fork port that hands each connection its own forked database.
// Configuration follows the upstream postgres image — POSTGRES_USER,
// POSTGRES_PASSWORD, POSTGRES_DB, PGPORT — so the bundled petri:postgres
// image is a drop-in replacement on the passthrough port.
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
	PassthroughAddr string // PETRI_PASSTHROUGH_ADDR; default ":5432"
	ForkAddr        string // PETRI_FORK_ADDR;        default ":5433"
	BackendPort     string // PGPORT; default "5432" (bundled image sets it)
	AdminUser       string // POSTGRES_USER; default "postgres"
	AdminPassword   string // POSTGRES_PASSWORD
}

func loadConfig(getenv func(string) string) config {
	return config{
		PassthroughAddr: envOr(getenv, "PETRI_PASSTHROUGH_ADDR", ":5432"),
		ForkAddr:        envOr(getenv, "PETRI_FORK_ADDR", ":5433"),
		BackendPort:     envOr(getenv, "PGPORT", "5432"),
		AdminUser:       envOr(getenv, "POSTGRES_USER", "postgres"),
		AdminPassword:   getenv("POSTGRES_PASSWORD"),
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
	backendAddr := "127.0.0.1:" + cfg.BackendPort

	ptLn, err := net.Listen("tcp", cfg.PassthroughAddr)
	if err != nil {
		return fmt.Errorf("listen passthrough %s: %w", cfg.PassthroughAddr, err)
	}
	fkLn, err := net.Listen("tcp", cfg.ForkAddr)
	if err != nil {
		ptLn.Close()
		return fmt.Errorf("listen fork %s: %w", cfg.ForkAddr, err)
	}
	fmt.Fprintf(logs, "petri listening: passthrough=%s fork=%s -> backend=%s\n",
		ptLn.Addr(), fkLn.Addr(), backendAddr)

	passthrough := &proxy.Proxy{BackendAddr: backendAddr}
	forking := &proxy.Proxy{
		BackendAddr: backendAddr,
		OnStartup:   forkPerConnection(cfg, forker.Forker{}, logs),
	}

	errs := make(chan error, 2)
	go func() { errs <- passthrough.Serve(ptLn) }()
	go func() { errs <- forking.Serve(fkLn) }()

	// First listener to return determines the exit status; close the other
	// so its goroutine unblocks and we can drain the second result.
	first := <-errs
	ptLn.Close()
	fkLn.Close()
	<-errs
	return first
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
