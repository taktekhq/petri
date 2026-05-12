package proxy_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/taktekhq/petri/proxy"
)

const (
	pgDatabase = "appdb"
	pgUser     = "appuser"
	pgPassword = "apppass"
)

// TestProxy_SelectOne is the wire-level smoke test: a real pgx client through
// the proxy can complete a query against real Postgres.
func TestProxy_SelectOne(t *testing.T) {
	addr := startProxyToPostgres(t)
	db := openPGX(t, addr)

	var n int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&n))
	require.Equal(t, 1, n)
}

// TestProxy_ParallelConnections proves connections don't interfere — the
// foundation that the forking work depends on.
func TestProxy_ParallelConnections(t *testing.T) {
	addr := startProxyToPostgres(t)

	for i := 0; i < 8; i++ {
		i := i
		t.Run(fmt.Sprintf("conn-%d", i), func(t *testing.T) {
			t.Parallel()
			db := openPGX(t, addr)
			var got int
			require.NoError(t, db.QueryRow("SELECT $1::int", i).Scan(&got))
			require.Equal(t, i, got)
		})
	}
}

// TestProxy_ServeReturnsOnListenerClose pins the shutdown contract that test
// cleanup and `main` rely on.
func TestProxy_ServeReturnsOnListenerClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := serveAsync(&proxy.Proxy{BackendAddr: "127.0.0.1:1"}, ln)
	require.NoError(t, ln.Close())

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after listener close")
	}
}

// ---- helpers ----

// startProxyToPostgres boots a fresh Postgres + proxy pair and returns the
// proxy's listen address. One call per test keeps the data isolated.
func startProxyToPostgres(t *testing.T) string {
	t.Helper()
	backend := startPostgres(t)
	return startProxy(t, backend)
}

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(pgDatabase),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	host, err := pg.Host(ctx)
	require.NoError(t, err)
	port, err := pg.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	return net.JoinHostPort(host, port.Port())
}

func startProxy(t *testing.T, backendAddr string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go (&proxy.Proxy{BackendAddr: backendAddr}).Serve(ln)
	return ln.Addr().String()
}

func openPGX(t *testing.T, addr string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		pgUser, pgPassword, addr, pgDatabase)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// serveAsync runs Serve in a goroutine, returning a channel for the result.
func serveAsync(p *proxy.Proxy, ln net.Listener) <-chan error {
	out := make(chan error, 1)
	go func() { out <- p.Serve(ln) }()
	return out
}
