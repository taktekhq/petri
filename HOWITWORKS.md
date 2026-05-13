# How petri works

## Ports

On `:5432`, petri parses each client's `StartupMessage` and forwards the connection to the backend verbatim — same database, no extra overhead beyond a TCP hop.

On `:5433`, petri parses the same message, then runs `CREATE DATABASE petri_<uuid> TEMPLATE <db>`, rewrites the `StartupMessage` to point at the new fork, and proxies the rest. On disconnect the fork is dropped. Admin work uses `POSTGRES_USER`/`POSTGRES_PASSWORD`; client queries flow with the client's own credentials, preserving permissions and RLS.

## Configuration

| Env var                  | Default    | Purpose                                  |
| ------------------------ | ---------- | ---------------------------------------- |
| `POSTGRES_USER`          | `postgres` | Admin user for CREATE/DROP DATABASE      |
| `POSTGRES_PASSWORD`      | (required) | Admin password                           |
| `POSTGRES_DB`            | `postgres` | Template DB the postgres image creates   |
| `PGPORT`                 | `5434`     | Loopback Postgres port (inside image)    |
| `PETRI_PASSTHROUGH_ADDR` | `:5432`    | Passthrough listener bind address        |
| `PETRI_FORK_ADDR`        | `:5433`    | Fork listener bind address               |

## Usage from tests

One TCP connection = one fork. Set `max: 1` on your pool.

**TypeScript + Knex**

```ts
knex({
  client: 'pg',
  connection: { host: 'db', port: 5433, user: 'appuser', password: 'apppass', database: 'appdb' },
  pool: { min: 1, max: 1 },
})
```

**Go (pgx)**

```go
// pgx.Connect (no pool) is the safest option.
conn, err := pgx.Connect(ctx, "postgres://appuser:apppass@db:5433/appdb?sslmode=disable")
```

If you use `database/sql`, set `db.SetMaxOpenConns(1)`.

## Limitations

- One TCP connection = one fork. Pools with `max > 1` break isolation.
- Plaintext only — petri rejects SSL/GSS to read the `StartupMessage`.
- Postgres only — relies on `CREATE DATABASE … TEMPLATE …`.
- `CREATE DATABASE` does a file-level copy. Multi-GB templates fork in seconds, not milliseconds. No warm pool yet.
