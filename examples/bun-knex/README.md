# bun + knex + petri example

Before/after demo: four tests, two outcomes.

```bash
docker compose run --rm app bun install
docker compose run --rm app bun run test:before   # 1 pass, 3 fail
docker compose run --rm app bun run test:after    # 4 pass, 0 fail
```

`test:before` hits `:5432` — shared DB, tests stomp on each other.
`test:after` hits `:5433` — each `withApp()` call gets its own fork.

Both scripts run `migrate` first so no manual reset is needed between runs.

To build petri locally instead of pulling the published image, swap `image:` for `build: ../..` in `docker-compose.yml`.
