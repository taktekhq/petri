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
	testDB   = "appdb"
	testUser = "appuser"
	testPass = "apppass"
)

// startBackend boots a Postgres container shared across the tests in this
// package. Returned address is host:port reachable from the test process.
func startBackend(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(testDB),
		postgres.WithUsername(testUser),
		postgres.WithPassword(testPass),
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

// startProxy launches a proxy.Proxy on a random local port pointing at backend
// and returns its listen address.
func startProxy(t *testing.T, backend string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	p := &proxy.Proxy{BackendAddr: backend}
	go func() { _ = p.Serve(ln) }()
	return ln.Addr().String()
}

func openDB(t *testing.T, addr string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		testUser, testPass, addr, testDB)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestProxyForwardsSelect1 is the smoke test: a real psql client connecting
// through the proxy can complete a query against the real Postgres backend.
// If this passes we know the bidirectional pipe is correct.
func TestProxyForwardsSelect1(t *testing.T) {
	backend := startBackend(t)
	proxyAddr := startProxy(t, backend)

	db := openDB(t, proxyAddr)

	var n int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&n))
	require.Equal(t, 1, n)
}

// TestProxyHandlesParallelConnections proves the goroutine-per-connection model
// works: many independent queries through the same proxy must not interfere.
// This is the property that the later forking work depends on.
func TestProxyHandlesParallelConnections(t *testing.T) {
	backend := startBackend(t)
	proxyAddr := startProxy(t, backend)

	const N = 8
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			db := openDB(t, proxyAddr)
			var got int
			err := db.QueryRow("SELECT $1::int", i).Scan(&got)
			if err == nil && got != i {
				err = fmt.Errorf("got %d want %d", got, i)
			}
			errs <- err
		}()
	}
	for i := 0; i < N; i++ {
		require.NoError(t, <-errs)
	}
}

// TestServeReturnsWhenListenerClosed pins the contract that Serve exits
// cleanly when its listener is closed — the cleanup pattern tests rely on.
func TestServeReturnsWhenListenerClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	p := &proxy.Proxy{BackendAddr: "127.0.0.1:1"} // unused; never accept
	done := make(chan error, 1)
	go func() { done <- p.Serve(ln) }()

	require.NoError(t, ln.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after listener close")
	}
}
