# petri

Isolated, seeded database environments for parallel testing.

Petri is a Postgres proxy in front of a real Postgres. When a client connects,
petri silently forks the database the client asked for into a UUID-named copy,
points the client at the fork, and drops the fork when the client disconnects.

The result: **every TCP connection sees its own private database**, seeded from
the same template, fully writeable, independent of every other connection.

Concretely:
- You set up your seed / migrations once (via the standard postgres
  `/docker-entrypoint-initdb.d/` mechanism).
- Your tests run in parallel — each opens a connection through petri and gets
  its own copy of the seeded database.
- Tests can `COMMIT` freely, drop tables, mutate schema — they don't see each
  other's writes.
- When the test ends and closes the connection, petri drops the fork.

There's nothing test-framework-specific about this. Petri speaks the regular
Postgres wire protocol; any client library works.

## Quickstart

In your `docker-compose.yml`, replace your Postgres image with petri's:

```yaml
services:
  db:
    image: petri:postgres
    environment:
      POSTGRES_USER: appuser
      POSTGRES_PASSWORD: apppass
      POSTGRES_DB: appdb
    # Seed / migrations land on the template database, so every fork inherits
    # them. Mount your existing init scripts or .sql files here.
    volumes:
      - ./db/migrations:/docker-entrypoint-initdb.d/:ro

  app:
    image: my-app
    depends_on: [db]
    environment:
      DATABASE_URL: postgres://appuser:apppass@db:5432/appdb
```

Build the image once (until petri is published to a registry):

```bash
git clone https://github.com/taktekhq/petri.git
cd petri
docker build -t petri:postgres .
```

Then `docker compose up`. Your app and tests talk to `db:5432` exactly as they
would with the upstream postgres image. The only difference is that every
client connection gets its own forked database.

## How it works (in one paragraph)

The petri:postgres image runs two processes:
1. A Postgres server, listening on the loopback port (`PGPORT`, default 5433
   inside the image).
2. The petri proxy, listening on the public port `5432` and forwarding to
   loopback Postgres.

When a client opens a TCP connection, petri reads the Postgres StartupMessage
(which includes the database the client asked for), opens an admin connection
to Postgres using `POSTGRES_USER` + `POSTGRES_PASSWORD`, runs
`CREATE DATABASE petri_<uuid> TEMPLATE <database>`, rewrites the
StartupMessage to point at the fork, then proxies the rest of the conversation
verbatim. When the TCP connection closes, petri runs `DROP DATABASE petri_<uuid>`.

Petri's own admin work always uses admin credentials, so forking succeeds even
when the client is a read-only role. The client's own queries flow through the
proxy with their own credentials, so permissions are preserved end-to-end.

## Configuration

Petri reads exactly the env vars the upstream postgres image already documents:

| Env var             | Default      | Used for                                    |
| ------------------- | ------------ | ------------------------------------------- |
| `POSTGRES_USER`     | `postgres`   | Admin user for `CREATE/DROP DATABASE`       |
| `POSTGRES_PASSWORD` | (required)   | Admin password                              |
| `POSTGRES_DB`       | `postgres`   | Template DB created at first boot           |
| `PGPORT`            | `5433`       | Loopback Postgres port (inside the image)   |

Optional petri override:

| Env var             | Default   | Used for                          |
| ------------------- | --------- | --------------------------------- |
| `PETRI_LISTEN_ADDR` | `:5432`   | Where petri itself listens        |

## Using petri from tests

The one rule: **each test case must use exactly one TCP connection.** A
connection pool with `max > 1` would multiplex queries across multiple
connections, each of which becomes its own fork — INSERTs on one would be
invisible to SELECTs on another. So:

- Create a fresh client (or pool with `max = 1`) per test.
- Use it for the whole test.
- Destroy it at the end so the fork is dropped.

### TypeScript + Knex

```ts
import knex, { type Knex } from 'knex';

function newTestDB(): Knex {
  return knex({
    client: 'pg',
    connection: {
      host: process.env.DB_HOST ?? 'db',
      port: 5432,
      user: 'appuser',
      password: 'apppass',
      database: 'appdb',
    },
    // CRITICAL: one connection per Knex instance, so every query in the test
    // lands on the same fork.
    pool: { min: 1, max: 1 },
  });
}

describe('user repository', () => {
  let db: Knex;

  beforeEach(() => {
    db = newTestDB();
  });

  afterEach(async () => {
    await db.destroy(); // closes the connection → petri drops the fork
  });

  it('finds the seeded user', async () => {
    const user = await db('users').where({ id: 1 }).first();
    expect(user?.name).toBe('alice');
  });

  it('does not see writes from a parallel test', async () => {
    // Another test in parallel might delete this row from its fork.
    // We have our own fork, so we still see it.
    expect(await db('users').where({ id: 1 }).first()).toBeDefined();
  });
});
```

With Jest/Vitest's default parallel test runner this works directly — each
worker process makes its own Knex instance, each gets its own fork.

### Go (database/sql with pgx)

```go
import (
    "database/sql"
    "testing"

    _ "github.com/jackc/pgx/v5/stdlib"
    "github.com/stretchr/testify/require"
)

const dsn = "postgres://appuser:apppass@db:5432/appdb?sslmode=disable"

// openTestDB returns a *sql.DB constrained to a single TCP connection, so
// every query in a test lands on the same fork.
func openTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("pgx", dsn)
    require.NoError(t, err)
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    t.Cleanup(func() { _ = db.Close() })
    return db
}

func TestFindsSeededUser(t *testing.T) {
    t.Parallel()
    db := openTestDB(t)

    var name string
    require.NoError(t, db.QueryRow(
        "SELECT name FROM users WHERE id = $1", 1).Scan(&name))
    require.Equal(t, "alice", name)
}

func TestIsolatedFromParallelDeletes(t *testing.T) {
    t.Parallel()
    db := openTestDB(t)

    // Even if another parallel test deleted this row, we have our own fork.
    var exists bool
    require.NoError(t, db.QueryRow(
        "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", 1).Scan(&exists))
    require.True(t, exists)
}
```

### Go (pgx directly, for maximum control)

```go
import (
    "context"

    "github.com/jackc/pgx/v5"
)

func openTestConn(t *testing.T) *pgx.Conn {
    conn, err := pgx.Connect(context.Background(), dsn)
    require.NoError(t, err)
    t.Cleanup(func() { _ = conn.Close(context.Background()) })
    return conn
}
```

`pgx.Conn` is a single connection by construction — no pool to misconfigure.

### Other clients (general principle)

The pattern is the same regardless of library: open one TCP connection per
test, hold it for the whole test, close it at the end. If your library has a
pool, set `max = 1`. If it auto-reconnects on error, prefer a single explicit
connection over the pool to avoid landing on a fresh fork mid-test.

## Migrations / seed data

Anything you'd normally put under `/docker-entrypoint-initdb.d/` works
unchanged. The standard postgres image runs every `*.sql`, `*.sql.gz`,
`*.sh`, etc. once when the data directory is first initialised. Petri inherits
this — those scripts run against the template database before petri starts,
and every fork later inherits the result.

```yaml
services:
  db:
    image: petri:postgres
    environment:
      POSTGRES_USER: appuser
      POSTGRES_PASSWORD: apppass
      POSTGRES_DB: appdb
    volumes:
      - ./migrations:/docker-entrypoint-initdb.d/:ro
```

For application-managed migrations (e.g. Knex's `migrate:latest`, sql-migrate,
golang-migrate), run them **once** against the template before tests start.
The simplest pattern in CI:

```sh
docker compose up -d db
# wait for petri to be ready (it logs "petri listening" when it is)
until docker compose logs db | grep -q "petri listening"; do sleep 0.5; done

# run migrations directly against loopback Postgres inside the container,
# so they land on the template (not a fork):
docker compose exec -T db psql \
  -h 127.0.0.1 -p 5433 -U appuser -d appdb \
  -f /migrations/000_init.sql
```

Note: if you run your migrations *through* petri's exposed port (`5432`), they
land on a fork, not the template. Use the loopback port (`5433`) inside the
container for one-shot template setup, or — preferred — use
`/docker-entrypoint-initdb.d/` so postgres handles it during init.

## Running petri locally for development

You need Go 1.25+ and Docker.

```bash
git clone https://github.com/taktekhq/petri.git
cd petri

# Run the unit + integration tests (fast, ~30s).
go test -short ./...

# Run everything including end-to-end image tests (~2 min, rebuilds the image).
go test -timeout=15m ./...

# Build and run the bundled image manually.
docker build -t petri:postgres .
docker run --rm -p 5432:5432 \
  -e POSTGRES_USER=appuser \
  -e POSTGRES_PASSWORD=apppass \
  -e POSTGRES_DB=appdb \
  petri:postgres

# In another terminal:
psql postgres://appuser:apppass@127.0.0.1:5432/appdb \
  -c "SELECT current_database();"
# → petri_<uuid>
```

## Tests

| Suite                  | What it exercises                            | Approx. time | Needs Docker |
| ---------------------- | -------------------------------------------- | ------------ | ------------ |
| `internal/startup/...` | StartupMessage parsing, SSL/GSS rejection    | <50 ms       | no           |
| `internal/proxy/...`   | bridge + real Postgres via testcontainers    | ~15 s        | yes          |
| `internal/forker/...`  | CREATE/DROP DATABASE via testcontainers      | ~15 s        | yes          |
| `cmd/petri/...`        | config, forkPerConnection with a fake forker | <50 ms       | no           |
| `e2e/...`              | builds petri:test image, real client → fork  | ~75 s        | yes          |

```bash
# Skip the slow image-build suite during inner-loop iteration.
go test -short ./...

# Just the one e2e test that proves parallel isolation.
go test -v -timeout=10m -run TestImage_ParallelWorkersSeeIndependentForks ./e2e/...
```

## Limitations

- **One TCP connection = one fork.** Library connection pools with `max > 1`
  will split queries across multiple forks and break test invariants.
- **Plaintext only.** Petri rejects SSL/GSS handshakes so it can inspect and
  rewrite the StartupMessage. Run petri only on trusted networks (loopback,
  internal docker networks).
- **Postgres only.** No MySQL/SQLite/etc. support; the design is Postgres-
  specific because it relies on `CREATE DATABASE … TEMPLATE …`.
- **Heavy templates take time to fork.** `CREATE DATABASE` does a file-level
  copy, so a multi-GB seeded template means multi-second forks. Petri does
  not warm-pool forks today.

## License

TBD.
