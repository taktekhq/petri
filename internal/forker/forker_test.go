package forker_test

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

	"github.com/taktekhq/petri/internal/forker"
)

const (
	pgUser     = "appuser"
	pgPassword = "apppass"
	template   = "appdb"
)

// TestForker_ForkCopiesTemplate is the headline test: seed rows in the
// template, fork, and assert the fork sees those rows.
func TestForker_ForkCopiesTemplate(t *testing.T) {
	pg := startPostgres(t)
	seedTemplate(t, pg, "CREATE TABLE seeded (n int)", "INSERT INTO seeded VALUES (42)")

	f := forker.Forker{}
	require.NoError(t, f.Fork(ctx(t), pg.dsn("postgres"), template, "fork_one"))

	var got int
	require.NoError(t, openSQL(t, pg.dsn("fork_one")).
		QueryRow("SELECT n FROM seeded").Scan(&got))
	require.Equal(t, 42, got)
}

// TestForker_ForksAreIndependent proves writes to one fork don't reach the
// template or other forks.
func TestForker_ForksAreIndependent(t *testing.T) {
	pg := startPostgres(t)
	seedTemplate(t, pg, "CREATE TABLE t (n int)")

	f := forker.Forker{}
	require.NoError(t, f.Fork(ctx(t), pg.dsn("postgres"), template, "fork_a"))
	require.NoError(t, f.Fork(ctx(t), pg.dsn("postgres"), template, "fork_b"))

	openSQL(t, pg.dsn("fork_a")).Exec("INSERT INTO t VALUES (1)")
	openSQL(t, pg.dsn("fork_b")).Exec("INSERT INTO t VALUES (2)")

	require.Equal(t, 1, countRows(t, openSQL(t, pg.dsn("fork_a")), "t"))
	require.Equal(t, 1, countRows(t, openSQL(t, pg.dsn("fork_b")), "t"))
	require.Equal(t, 0, countRows(t, openSQL(t, pg.dsn(template)), "t"))
}

// TestForker_Drop removes a fork.
func TestForker_Drop(t *testing.T) {
	pg := startPostgres(t)
	seedTemplate(t, pg, "CREATE TABLE t (n int)")

	f := forker.Forker{}
	require.NoError(t, f.Fork(ctx(t), pg.dsn("postgres"), template, "fork_to_drop"))
	require.NoError(t, f.Drop(ctx(t), pg.dsn("postgres"), "fork_to_drop"))

	_, err := openSQL(t, pg.dsn("fork_to_drop")).Exec("SELECT 1")
	require.ErrorContains(t, err, "does not exist")
}

// TestForker_DropIsIdempotent — calling Drop twice doesn't error.
func TestForker_DropIsIdempotent(t *testing.T) {
	pg := startPostgres(t)
	f := forker.Forker{}
	require.NoError(t, f.Drop(ctx(t), pg.dsn("postgres"), "never_existed"))
}

// ---- helpers ----

type pgContainer struct{ host, port string }

func (p pgContainer) dsn(db string) string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		pgUser, pgPassword, net.JoinHostPort(p.host, p.port), db)
}

func startPostgres(t *testing.T) pgContainer {
	t.Helper()
	c := context.Background()
	pg, err := postgres.Run(c,
		"postgres:16.4-alpine",
		postgres.WithDatabase(template),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(c) })

	host, err := pg.Host(c)
	require.NoError(t, err)
	port, err := pg.MappedPort(c, "5432/tcp")
	require.NoError(t, err)
	return pgContainer{host: host, port: port.Port()}
}

// seedTemplate runs DDL/DML against the template, then closes the connection
// pool so the template has no active sessions when Fork runs.
func seedTemplate(t *testing.T, pg pgContainer, statements ...string) {
	t.Helper()
	db, err := sql.Open("pgx", pg.dsn(template))
	require.NoError(t, err)
	defer db.Close()
	for _, s := range statements {
		_, err := db.Exec(s)
		require.NoError(t, err, "seed: %s", s)
	}
}

func openSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n))
	return n
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}
