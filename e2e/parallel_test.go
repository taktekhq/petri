package e2e_test

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// schemaWithSeed is the migration that each parallel worker's fork starts
// from — three rows in a single table. Real-world tests would have many
// more tables and rows; the principle is the same.
const schemaWithSeed = `
CREATE TABLE widgets (
    id   int PRIMARY KEY,
    name text NOT NULL
);
INSERT INTO widgets VALUES
    (1, 'gizmo'),
    (2, 'gadget'),
    (3, 'doohickey');
`

// TestImage_ParallelWorkersSeeIndependentForks is the realistic test: one
// petri:postgres container is seeded once via /docker-entrypoint-initdb.d/,
// then N test workers run in parallel, each holding its own DB connection.
// Half of them DELETE a row; the other half should still see the full seed.
//
// This is exactly the scenario petri is built for — fast parallel DB tests
// against a seeded template.
func TestImage_ParallelWorkersSeeIndependentForks(t *testing.T) {
	skipIfShort(t)

	addrs := startPetriImage(t, schemaWithSeed)

	const workers = 8
	for i := 0; i < workers; i++ {
		i := i
		t.Run(fmt.Sprintf("worker-%02d", i), func(t *testing.T) {
			t.Parallel()
			db := openPGX(t, addrs.fork, fmt.Sprintf("worker-%02d", i))

			// Every fork starts from the seeded template.
			require.Equal(t, 3, scanInt(t, db, "SELECT count(*) FROM widgets"),
				"fork should start with seeded rows")

			if i%2 == 0 {
				// Even workers delete row 1 from THEIR fork. The original
				// template and other forks are unaffected.
				mustExec(t, db, "DELETE FROM widgets WHERE id = 1")
				require.Equal(t, 2, scanInt(t, db, "SELECT count(*) FROM widgets"),
					"deleter should see 2 rows post-delete")
				require.False(t, rowExists(t, db, 1),
					"deleter should not see row id=1 after deletion")
			} else {
				// Odd workers don't touch the data. They should still see
				// the full seed — independence from the deleters proves the
				// per-connection fork is real, not shared.
				require.True(t, rowExists(t, db, 1),
					"non-deleter should still see row id=1")
				require.True(t, rowExists(t, db, 2))
				require.True(t, rowExists(t, db, 3))
			}
		})
	}
}

// TestImage_ParallelWorkersCanWriteSameTableName proves there is no name
// collision between forks: every worker creates the same-named table and
// inserts its own row, and only sees its own.
func TestImage_ParallelWorkersCanWriteSameTableName(t *testing.T) {
	skipIfShort(t)

	addrs := startPetriImage(t, "")

	const workers = 8
	for i := 0; i < workers; i++ {
		i := i
		t.Run(fmt.Sprintf("worker-%02d", i), func(t *testing.T) {
			t.Parallel()
			db := openPGX(t, addrs.fork, fmt.Sprintf("worker-%02d", i))

			mustExec(t, db, "CREATE TABLE only_mine (n int)")
			mustExec(t, db, fmt.Sprintf("INSERT INTO only_mine VALUES (%d)", i))

			require.Equal(t, i, scanInt(t, db, "SELECT n FROM only_mine"))
			require.Equal(t, 1, scanInt(t, db, "SELECT count(*) FROM only_mine"))
		})
	}
}

func rowExists(t *testing.T, db *sql.DB, id int) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM widgets WHERE id = $1)`, id,
	).Scan(&exists))
	return exists
}
