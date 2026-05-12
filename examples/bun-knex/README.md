# bun + knex + petri example

Tiny CRUD API over three tables (`users`, `stores`, `products`). One
shared `crud()` factory builds every router; each resource is a one-line
file. Tests run in parallel across Bun workers, each test on its own
petri fork.

## Layout

```
docker-compose.yml             # postgres (petri) + app (bun) — no ports
src/
  db.ts                        # newDB() — single source of truth
  crud.ts                      # crud(table, fields) → router factory
  app.ts                       # mounts users/stores/products
  server.ts                    # `bun run start`
  migrate.ts                   # drop + create + seed in one shot
  resources/
    users.ts                   # one line: crud('users', [...])
    stores.ts
    products.ts
test/
  helpers.ts                   # withApp() + send() — fork-per-test
  users.test.ts
  stores.test.ts
  products.test.ts
```

## Run it

Everything runs inside compose — no ports are published to the host.
The postgres service sets `POSTGRES_USER=postgres` and
`POSTGRES_PASSWORD=postgres` (the familiar postgres-image defaults);
`src/db.ts` falls back to the same values, so no other env vars are
needed.

```bash
docker compose run --rm app bun install
docker compose run --rm app bun run migrate    # PGPORT=5432, seeds template
docker compose run --rm app bun test           # PGPORT=5433, fork per test
docker compose run --rm --service-ports app bun run start   # if you want HTTP
```

The compose builds the petri image from the repo root on first
`docker compose up`. To pull a published build instead, swap the active
`build:` line for the commented `image:` one above it.

## How parallel isolation works

- `bun test` runs each `*.test.ts` file in its own worker.
- Inside every test, `withApp()` opens one TCP connection to
  `postgres:5433` — petri runs `CREATE DATABASE … TEMPLATE postgres`,
  the connection lands on that fork, and `db.destroy()` closes it so
  petri drops the fork.
- `pool: { min: 1, max: 1 }` is critical: one connection = one fork.
- `migrate` and `start` use the default `PGPORT=5432` (passthrough);
  the `test` script sets `PGPORT=5433`. No code branches on this — the
  env var alone routes traffic to the right petri port.
