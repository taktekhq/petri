# petri

A Postgres proxy that gives each TCP client its own forked database.
Drop-in replacement for `postgres:16.4-alpine` for parallel-test workloads.

In docker-compose, swap the image:

```yaml
db:
  image: petri:postgres
  environment:
    POSTGRES_USER: appuser
    POSTGRES_PASSWORD: apppass
    POSTGRES_DB: appdb
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

Every TCP connection triggers `CREATE DATABASE petri_<uuid> TEMPLATE <db>`;
petri rewrites the StartupMessage to point at the fork and proxies the
rest verbatim. On disconnect, the fork is dropped. The proxy uses
`POSTGRES_USER`/`POSTGRES_PASSWORD` for admin work, so forking succeeds
even for read-only clients; client queries flow with the client's own
credentials, preserving permissions and RLS.

## Configuration

| Env var             | Default    | Used for                                |
| ------------------- | ---------- | --------------------------------------- |
| `POSTGRES_USER`     | `postgres` | Admin user for CREATE/DROP DATABASE     |
| `POSTGRES_PASSWORD` | (required) | Admin password                          |
| `POSTGRES_DB`       | `postgres` | Template DB the postgres image creates  |
| `PGPORT`            | `5433`     | Loopback Postgres port (inside image)   |
| `PETRI_LISTEN_ADDR` | `:5432`    | Where petri listens                     |

## Using it from tests

**One TCP connection per test.** Pools with `max > 1` split queries across
multiple forks and break isolation.

### TypeScript + Knex

```ts
import knex, { type Knex } from 'knex';

function newTestDB(): Knex {
  return knex({
    client: 'pg',
    connection: { host: 'db', user: 'appuser', password: 'apppass', database: 'appdb' },
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
const dsn = "postgres://appuser:apppass@db:5432/appdb?sslmode=disable"

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

App-managed migrations (knex, golang-migrate, etc.) must run against the
template — connect via the loopback port `5433` inside the container, not
through petri on `5432` (which would fork them).

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
