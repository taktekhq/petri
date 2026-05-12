# petri

A Postgres proxy with two listeners on one Postgres backend:

- **`5432` — passthrough.** Behaves like regular Postgres. Use this for
  dev, migrations, and seed scripts.
- **`5433` — fork-per-connection.** Every TCP client lands on a fresh
  `CREATE DATABASE … TEMPLATE <db>` and the fork is dropped on disconnect.
  Use this for parallel test workers.

Same image, same data, two surfaces — pick the port that matches what you
want to do. Drop-in replacement for `postgres:16.4-alpine` on `5432`.

In docker-compose, swap the image:

```yaml
db:
  image: petri:postgres
  environment:
    POSTGRES_USER: appuser
    POSTGRES_PASSWORD: apppass
    POSTGRES_DB: appdb
  ports:
    - "5432:5432"   # drop-in postgres
    - "5433:5433"   # fork-per-connection (optional — only if tests reach it)
```

Or in your own Dockerfile, swap the base:

```Dockerfile
FROM petri:postgres
COPY init.sh /docker-entrypoint-initdb.d/
```

Build it once until it's published:

```bash
docker build -t petri:postgres .
```

Postgres 16.4-alpine is the only supported base.

## How it works

On `5432`, petri parses each client's StartupMessage and forwards the
connection to the backend verbatim — same database, same data, no extra
overhead beyond a TCP hop.

On `5433`, petri does the same parse, then runs
`CREATE DATABASE petri_<uuid> TEMPLATE <db>`, rewrites the StartupMessage
to point at the new fork, and proxies the rest. On disconnect the fork is
dropped. The proxy uses `POSTGRES_USER`/`POSTGRES_PASSWORD` for admin work,
so forking succeeds even for read-only clients; client queries flow with
the client's own credentials, preserving permissions and RLS.

## Configuration

| Env var                  | Default    | Used for                                |
| ------------------------ | ---------- | --------------------------------------- |
| `POSTGRES_USER`          | `postgres` | Admin user for CREATE/DROP DATABASE     |
| `POSTGRES_PASSWORD`      | (required) | Admin password                          |
| `POSTGRES_DB`            | `postgres` | Template DB the postgres image creates  |
| `PGPORT`                 | `5434`     | Loopback Postgres port (inside image)   |
| `PETRI_PASSTHROUGH_ADDR` | `:5432`    | Where petri's passthrough listener binds |
| `PETRI_FORK_ADDR`        | `:5433`    | Where petri's fork listener binds       |

## Using it from tests

Connect tests to the **fork port (5433)**. App code, migrations, and
seeds connect to the **passthrough port (5432)** — same as plain postgres.

**One TCP connection per test.** Pools with `max > 1` split queries across
multiple forks and break isolation.

### TypeScript + Knex

```ts
import knex, { type Knex } from 'knex';

function newTestDB(): Knex {
  return knex({
    client: 'pg',
    connection: { host: 'db', port: 5433, user: 'appuser', password: 'apppass', database: 'appdb' },
    pool: { min: 1, max: 1 },          // critical
  });
}

describe('users', () => {
  let db: Knex;
  beforeEach(() => { db = newTestDB(); });
  afterEach(async () => { await db.destroy(); });   // closes → fork dropped

  it('finds the seed', async () => {
    expect(await db('users').where({ id: 1 }).first()).toBeDefined();
  });
});
```

### Go (database/sql + pgx)

```go
const dsn = "postgres://appuser:apppass@db:5433/appdb?sslmode=disable"

func openTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("pgx", dsn)
    require.NoError(t, err)
    db.SetMaxOpenConns(1)                // critical
    db.SetMaxIdleConns(1)
    t.Cleanup(func() { _ = db.Close() })
    return db
}

func TestFindsSeed(t *testing.T) {
    t.Parallel()
    var exists bool
    require.NoError(t, openTestDB(t).QueryRow(
        "SELECT EXISTS(SELECT 1 FROM users WHERE id = 1)").Scan(&exists))
    require.True(t, exists)
}
```

For Go, `pgx.Connect` (no pool) is even safer.

## Migrations / seed data

Anything under `/docker-entrypoint-initdb.d/` runs once against the template
before petri starts. Every fork inherits it.

App-managed migrations (knex, golang-migrate, etc.) connect to the
**passthrough port (5432)** — the writes land on the real template DB and
subsequent forks (on 5433) inherit them.

## Tests

```bash
go test -short ./...                  # ~30s, no Docker build
go test -timeout=15m ./...            # full incl. e2e
go test -run TestImage_Parallel ./e2e/...
```

| Suite                | Time   | Docker |
| -------------------- | ------ | ------ |
| `internal/startup`   | <50ms  | no     |
| `internal/proxy`     | ~15s   | yes    |
| `internal/forker`    | ~15s   | yes    |
| `cmd/petri`          | <50ms  | no     |
| `e2e/`               | ~75s   | yes    |

## Limitations

- One TCP connection = one fork. Pools with `max > 1` break isolation.
- Plaintext only. Petri rejects SSL/GSS so it can read the StartupMessage.
- Postgres only — relies on `CREATE DATABASE … TEMPLATE …`.
- `CREATE DATABASE` does a file-level copy; multi-GB templates fork in
  seconds, not milliseconds. No warm pool yet.
