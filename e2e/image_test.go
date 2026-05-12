// Package e2e tests the petri:postgres docker image end-to-end: it builds
// the image from the repo's Dockerfile, runs it like a user would, and
// drives it through real pgx clients.
//
// These tests are slower than the unit/integration tests in internal/ because
// they go all the way through the Docker build path. They are skipped under
// `go test -short`.
package e2e_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	pgUser     = "appuser"
	pgPassword = "apppass"
	pgDatabase = "appdb"
)

// TestImage_SmokeSelectOne is the first thing to break if the entrypoint or
// the bundled binary is wrong: a single client must be able to query through
// the drop-in (passthrough) port.
func TestImage_SmokeSelectOne(t *testing.T) {
	skipIfShort(t)
	addrs := startPetriImage(t, "")

	db := openPGX(t, addrs.passthrough, "")
	var n int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&n))
	require.Equal(t, 1, n)
}

// TestImage_PassthroughSharesOneDatabase pins the drop-in contract: clients
// on the passthrough port all see the same database — a write from one
// connection is visible on the next. This is what makes petri:postgres a
// transparent replacement for postgres on the standard 5432.
func TestImage_PassthroughSharesOneDatabase(t *testing.T) {
	skipIfShort(t)
	addrs := startPetriImage(t, "")

	writer := openPGX(t, addrs.passthrough, "writer")
	mustExec(t, writer, "CREATE TABLE shared (n int)")
	mustExec(t, writer, "INSERT INTO shared VALUES (42)")
	require.NoError(t, writer.Close())

	reader := openPGX(t, addrs.passthrough, "reader")
	require.Equal(t, 42, scanInt(t, reader, "SELECT n FROM shared"))
}

// TestImage_ForkPortIsolatesConnections is the user-visible forking contract
// on the shipping artifact: two clients on the fork port land on independent
// forks and can't see each other's writes.
func TestImage_ForkPortIsolatesConnections(t *testing.T) {
	skipIfShort(t)
	addrs := startPetriImage(t, "")

	a := openPGX(t, addrs.fork, "client-a")
	b := openPGX(t, addrs.fork, "client-b")

	mustExec(t, a, "CREATE TABLE t (id int)")
	mustExec(t, a, "INSERT INTO t VALUES (1)")
	mustExec(t, b, "CREATE TABLE t (id int)")
	mustExec(t, b, "INSERT INTO t VALUES (2)")

	require.Equal(t, 1, scanInt(t, a, "SELECT id FROM t"))
	require.Equal(t, 2, scanInt(t, b, "SELECT id FROM t"))
}

// TestImage_ForkPortDropsForkOnDisconnect: closing a fork-port client causes
// its fork to disappear from pg_database soon after.
func TestImage_ForkPortDropsForkOnDisconnect(t *testing.T) {
	skipIfShort(t)
	addrs := startPetriImage(t, "")

	a := openPGX(t, addrs.fork, "")
	var aFork string
	require.NoError(t, a.QueryRow("SELECT current_database()").Scan(&aFork))
	require.True(t, strings.HasPrefix(aFork, "petri_"), "expected petri_ prefix, got %q", aFork)

	require.NoError(t, a.Close())

	// Open a new fork-port connection (lands on a different fork) and use it
	// to poll pg_database for aFork's disappearance.
	probe := openPGX(t, addrs.fork, "")
	require.Eventually(t, func() bool {
		return !databaseExists(t, probe, aFork)
	}, 10*time.Second, 200*time.Millisecond, "fork %q was not dropped", aFork)
}

// ---- helpers ----

// petriAddrs is the pair of host:port addresses petri exposes: the
// passthrough port (drop-in postgres surface) and the fork port (a fresh
// forked database per connection).
type petriAddrs struct {
	passthrough string
	fork        string
}

// startPetriImage builds the image (cached after first run) and starts a
// container, returning the passthrough and fork addresses reachable from
// the test process. If initSQL is non-empty it is written into
// /docker-entrypoint-initdb.d/ so Postgres runs it once during init — the
// schema/seed lands on the template database that subsequent forks copy
// from.
func startPetriImage(t *testing.T, initSQL string) petriAddrs {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    repoRoot(t),
			Dockerfile: "Dockerfile",
			Repo:       "petri",
			Tag:        "test",
			KeepImage:  true,
		},
		ExposedPorts: []string{"5432/tcp", "5433/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     pgUser,
			"POSTGRES_PASSWORD": pgPassword,
			"POSTGRES_DB":       pgDatabase,
		},
		WaitingFor: wait.ForLog("petri listening").WithStartupTimeout(2 * time.Minute),
	}
	if initSQL != "" {
		req.Files = append(req.Files, testcontainers.ContainerFile{
			Reader:            strings.NewReader(initSQL),
			ContainerFilePath: "/docker-entrypoint-initdb.d/01-seed.sql",
			FileMode:          0o644,
		})
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	passthroughPort, err := c.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	forkPort, err := c.MappedPort(ctx, "5433/tcp")
	require.NoError(t, err)
	return petriAddrs{
		passthrough: net.JoinHostPort(host, passthroughPort.Port()),
		fork:        net.JoinHostPort(host, forkPort.Port()),
	}
}

// repoRoot finds the repo root by walking up from this source file. We can't
// rely on the working directory because `go test ./e2e/...` runs from the
// package dir, but the Dockerfile lives one level up.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(filepath.Dir(file)) // e2e/x.go → e2e → repo root
}

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

func databaseExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, name,
	).Scan(&exists))
	return exists
}

// shortFlagOnce is checked once to avoid repeated calls to testing.Short().
var shortFlagOnce sync.Once

func skipIfShort(t *testing.T) {
	t.Helper()
	shortFlagOnce.Do(func() {})
	if testing.Short() {
		t.Skip("e2e: skipping under -short (builds the petri:test image)")
	}
}
