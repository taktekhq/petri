package proxy_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/taktekhq/petri/internal/forker"
	"github.com/taktekhq/petri/internal/proxy"
	"github.com/taktekhq/petri/internal/startup"
)

const (
	pgDatabase = "appdb"
	pgUser     = "appuser"
	pgPassword = "apppass"
)

// TestProxy_SelectOne is the wire-level smoke test: a real pgx client through
// the proxy can complete a query against real Postgres.
func TestProxy_SelectOne(t *testing.T) {
	addr := startProxyToPostgres(t, nil)
	db := openPGX(t, addr, "")

	var n int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&n))
	require.Equal(t, 1, n)
}

// TestProxy_ParallelConnections proves connections don't interfere — the
// foundation that the forking work depends on.
func TestProxy_ParallelConnections(t *testing.T) {
	addr := startProxyToPostgres(t, nil)

	for i := 0; i < 8; i++ {
		i := i
		t.Run(fmt.Sprintf("conn-%d", i), func(t *testing.T) {
			t.Parallel()
			db := openPGX(t, addr, "")
			var got int
			require.NoError(t, db.QueryRow("SELECT $1::int", i).Scan(&got))
			require.Equal(t, i, got)
		})
	}
}

// TestProxy_OnStartup_CapturesApplicationName is Phase 2's headline test:
// a real pgx client connects with application_name=… and the hook sees it.
func TestProxy_OnStartup_CapturesApplicationName(t *testing.T) {
	var captured *startup.Info
	addr := startProxyToPostgres(t, func(i *startup.Info) error {
		captured = i
		return nil
	})

	db := openPGX(t, addr, "my-test-app")
	require.NoError(t, db.Ping())

	require.NotNil(t, captured, "OnStartup hook was never invoked")
	require.Equal(t, "my-test-app", captured.ApplicationName)
	require.Equal(t, pgDatabase, captured.Database)
	require.Equal(t, pgUser, captured.User)
}

// TestProxy_ForksDatabasePerConnection is Phase 3's headline test: when the
// OnStartup hook forks every client into a UUID-named copy of their requested
// database, two connections to the same logical database see fully independent
// tables. They can even create the same table name without conflict.
func TestProxy_ForksDatabasePerConnection(t *testing.T) {
	backendAddr := startPostgres(t)
	f := &forker.Forker{AdminDSN: adminDSN(backendAddr)}

	proxyAddr := serveProxy(t, &proxy.Proxy{
		BackendAddr: backendAddr,
		OnStartup:   forkIntoUUID(f),
	})

	a := openPGX(t, proxyAddr, "client-a")
	b := openPGX(t, proxyAddr, "client-b")

	mustExec(t, a, "CREATE TABLE t (id int)")
	mustExec(t, a, "INSERT INTO t VALUES (1)")
	mustExec(t, b, "CREATE TABLE t (id int)")
	mustExec(t, b, "INSERT INTO t VALUES (2)")

	require.Equal(t, 1, scanInt(t, a, "SELECT id FROM t"))
	require.Equal(t, 2, scanInt(t, b, "SELECT id FROM t"))
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

// startProxyToPostgres boots a fresh Postgres + proxy pair, attaches an
// optional OnStartup hook, and returns the proxy's listen address.
func startProxyToPostgres(t *testing.T, onStartup func(*startup.Info) error) string {
	t.Helper()
	return serveProxy(t, &proxy.Proxy{
		BackendAddr: startPostgres(t),
		OnStartup:   onStartup,
	})
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

func serveProxy(t *testing.T, p *proxy.Proxy) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go p.Serve(ln)
	return ln.Addr().String()
}

// openPGX opens a database/sql handle through the proxy. appName, if non-empty,
// sets the application_name connection parameter.
func openPGX(t *testing.T, addr, appName string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		pgUser, pgPassword, addr, pgDatabase)
	if appName != "" {
		dsn += "&application_name=" + appName
	}
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

// adminDSN returns a DSN that authenticates as the test superuser against the
// default "postgres" database — enough to run CREATE DATABASE.
func adminDSN(addr string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/postgres?sslmode=disable",
		pgUser, pgPassword, addr)
}

// forkIntoUUID is the minimal Phase 3 OnStartup hook: every client lands on
// a freshly-forked copy of their requested database, named by a UUID.
func forkIntoUUID(f *forker.Forker) func(*startup.Info) error {
	return func(i *startup.Info) error {
		name := "petri_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := f.Fork(ctx, i.Database, name); err != nil {
			return err
		}
		i.Database = name
		i.ApplicationName = name
		return nil
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	_, err := db.Exec(q)
	require.NoError(t, err)
}

func scanInt(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(q).Scan(&n))
	return n
}
