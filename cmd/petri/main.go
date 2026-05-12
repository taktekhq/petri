// Command petri starts the proxy. Configuration comes from environment
// variables so it drops into a docker-compose stack with no flag wiring.
package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"

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
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		ListenAddr:  getenv("PETRI_LISTEN_ADDR"),
		BackendAddr: getenv("PETRI_BACKEND_ADDR"),
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
	fmt.Fprintf(logs, "petri listening on %s, forwarding to %s\n", ln.Addr(), cfg.BackendAddr)

	p := &proxy.Proxy{
		BackendAddr: cfg.BackendAddr,
		OnStartup:   logConnections(logs),
	}
	return p.Serve(ln)
}

// logConnections returns an OnStartup hook that prints one line per client.
func logConnections(logs io.Writer) func(*startup.Info) error {
	return func(i *startup.Info) error {
		fmt.Fprintf(logs, "client connected: app=%q db=%q user=%q\n",
			i.ApplicationName, i.Database, i.User)
		return nil
	}
}
