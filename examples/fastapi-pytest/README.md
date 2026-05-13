# fastapi + pytest + petri example

Before/after demo: four tests, two outcomes.

```bash
make install
make before   # 2 pass, 2 fail
make after    # 4 pass
```

`make before` hits `:5432` — shared DB, tests stomp on each other.
`make after` hits `:5433` — each `client` fixture gets its own fork.

Both targets run `migrate` first so no manual reset is needed between runs.

`conftest.py` is the entire test infrastructure. No `CREATE DATABASE`, no teardown SQL, no transaction-rollback tricks. `engine.dispose()` drops the connection; petri drops the fork.

To build petri locally instead of pulling the published image, swap `image:` for `build: ../..` in `docker-compose.yml`.
