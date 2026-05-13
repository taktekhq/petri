# petri

Drop-in replacement for `postgres` that gives every test its own isolated database — no cleanup, no truncation, no flaky shared state.

```yaml
db:
  image: ghcr.io/taktekhq/petri-postgres:latest
  environment:
    POSTGRES_USER: appuser
    POSTGRES_PASSWORD: apppass
    POSTGRES_DB: appdb
  ports:
    - "5432:5432"   # passthrough — behaves like plain postgres
    - "5433:5433"   # fork-per-connection — each test gets its own copy
```

Point tests at `:5433`. Everything else (migrations, seeds, the app) stays on `:5432`.

```ts
knex({
  client: 'pg',
  connection: { host: 'db', port: 5433, ... },
  pool: { min: 1, max: 1 },   // one connection = one fork — required
})
```

## Migrations

Run against `:5432`. Writes land on the template; every fork inherits them.

## Examples

[`examples/bun-knex`](examples/bun-knex/) — before/after with Bun + Knex. Shows what breaks without isolation and what passes with it.

[`examples/fastapi-pytest`](examples/fastapi-pytest/) — before/after with FastAPI + pytest. Same pattern in Python.

## How it works

See [HOWITWORKS.md](HOWITWORKS.md).
