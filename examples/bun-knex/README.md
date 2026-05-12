# bun + knex + petri example

Tiny CRUD API over three tables (`users`, `stores`, `products`). One
shared `crud()` factory builds every router; each resource is a one-line
file. Tests run in parallel across Bun workers, each test on its own
petri fork.

## Layout

```
docker-compose.yml             # petri:postgres on :5432 + :5433
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

```bash
docker build -t petri:postgres ../..   # one-time
docker compose up -d
bun install
bun run migrate                         # PGPORT=5432, populates template
bun test                                # PGPORT=5433, fork per test
bun run start                           # http://localhost:3000
```

## How parallel isolation works

- `bun test` runs each `*.test.ts` file in its own worker.
- Inside every test, `withApp()` opens one TCP connection to `:5433` —
  petri runs `CREATE DATABASE … TEMPLATE appdb`, the connection lands
  on that fork, and `db.destroy()` closes it so petri drops the fork.
- `pool: { min: 1, max: 1 }` is critical: one connection = one fork.
- `migrate` and `start` use the default `PGPORT=5432` (passthrough);
  the `test` script sets `PGPORT=5433`. No code branches on this — the
  env var alone routes traffic to the right petri port.
