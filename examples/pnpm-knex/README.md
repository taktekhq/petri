# pnpm + knex + petri example

A small Express CRUD API over three tables (`users`, `stores`, `products`),
backed by Knex queries and tested in parallel against petri.

## Layout

```
docker-compose.yml             # petri:postgres on :5432 + :5433
knexfile.ts                    # passthrough connection for migrations/seeds
src/
  app.ts                       # mounts the routes
  server.ts                    # production entrypoint (port 5432)
  db/
    connection.ts              # dbConfig() + newDB() — pool max=1; PGPORT-driven
    migrations/                # users, stores, products
    seeds/                     # fixture rows every fork inherits
    queries/
      users.ts                 # one Knex module per resource
      stores.ts
      products.ts
  routes/
    users.ts                   # one Express router per resource
    stores.ts
    products.ts
test/
  helpers.ts                   # withTestApp() — fork-per-test plumbing
  users.test.ts                # one test file per resource — Jest runs
  stores.test.ts               # files in parallel; inside each file tests
  products.test.ts             # share the same forked DB.
```

## Run it

```bash
# 1. Build petri:postgres once (from repo root).
docker build -t petri:postgres ../..

# 2. Boot petri.
docker compose up -d

# 3. Install deps, migrate, seed.
pnpm install
pnpm migrate
pnpm seed

# 4. Run the parallel test suite.
pnpm test
```

The dev server (optional) listens on `:3000` and talks to the passthrough
port:

```bash
pnpm start
curl localhost:3000/users
```

## How parallel test isolation works

- App and migrations connect to `postgres:5432` (passthrough) — they read
  and write the real template DB. Seeds land here once.
- Tests connect to `postgres:5433` (fork-per-connection). Each TCP
  connection causes petri to run `CREATE DATABASE … TEMPLATE appdb` and
  rewrite the StartupMessage to point at the fork. On close, the fork
  is dropped.
- `pool: { min: 1, max: 1 }` is critical: one TCP connection = one fork.
  A pool with `max > 1` splits queries across multiple forks and breaks
  isolation.
- Jest runs each `*.test.ts` file in its own worker process, so the three
  files execute fully in parallel. Inside a file, each `it` wraps its
  work in its own `withTestApp` block, which means each test gets its
  own fork too — so writes never leak between tests, between files, or
  between Jest workers.

## Switching to a published image

If you publish `ghcr.io/taktekhq/petri:0.1-alpha` via the workflow in
`.github/workflows/publish-image.yml`, swap the `image:` line in
`docker-compose.yml` for it. The service name `postgres` stays the same,
so no app/test code changes are needed.
