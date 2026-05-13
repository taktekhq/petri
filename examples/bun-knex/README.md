# bun + knex + petri example

Minimal before/after demo. Two tables (`users` and `posts`, FK with
cascade), four tests, one clear result:

```bash
docker compose run --rm app bun install
docker compose run --rm app bun run test:before   # 1 pass, 3 fail
docker compose run --rm app bun run test:after    # 4 pass, 0 fail
```

The compose file pulls `ghcr.io/taktekhq/petri-postgres:16.4-0.1-alpha`.
To build from this checkout instead, swap `image:` for `build: ../..`.

## Why the before fails

`test:before` points at `:5432` (passthrough) — all tests share the
same database. The two parallel workers mutate the same data:

- `posts.test.ts` deletes Alice, which cascades to her posts.
  `users.test.ts` then sees Alice missing and fails its read.
- Both isolation tests (`deletes only inside its own fork`,
  `cascade delete stays inside its own fork`) open a second connection
  after a delete and expect to see the deleted row — it's gone for good
  on a shared database, so they fail.

`test:after` points at `:5433` (fork-per-connection) — every
`withApp()` call gets its own `CREATE DATABASE … TEMPLATE postgres`
copy. Deletes and cascades stay inside their fork. All four pass.

## Key constraint

`pool: { min: 1, max: 1 }` in `db.ts` is required. One TCP connection
= one fork. A pool with `max > 1` would spread queries across multiple
forks and break isolation.
