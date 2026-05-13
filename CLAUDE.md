# Petri — orientation for Claude

**Succinct is critical.** Short responses, no padding, no restating what the code already shows.

Postgres proxy with two listeners on one backend:
- `:5432` passthrough — transparent proxy, drop-in for `postgres`.
- `:5433` fork-per-connection — every TCP client gets its own `CREATE DATABASE … TEMPLATE …` copy, dropped on disconnect.

## Layout

```
cmd/petri/           main + forkPerConnection hook
internal/proxy/      Serve loop, handleClient, bridge
internal/startup/    StartupMessage parse + replay
internal/forker/     CREATE/DROP DATABASE
internal/adapters/   empty placeholder
internal/seed/       empty placeholder
docker/              entrypoint: backgrounds postgres, execs petri
e2e/                 builds petri:test, drives it via real pgx
examples/bun-knex/   before/after demo: bun + knex, users + posts FK
Dockerfile           golang:alpine builds petri → postgres:alpine bundles it
vendor/              gitignored; `go mod vendor` for offline builds
```

Every `*.go` has a matching `_test.go`.

## Per-connection flow (`internal/proxy/proxy.go`)

```
handleClient(client):
    startup.Read(client)       # parse, reject SSL/GSS
    OnStartup(info)            # fork port: forks DB, mutates info.Database
                               # passthrough port: hook is nil, info untouched
    net.Dial(BackendAddr)      # replay startup to backend
    bridge(client, backend)    # pipe until either side closes
    cleanup()                  # fork port only: drops the fork
```

`cmd/petri/main.go` spins two `proxy.Proxy` instances over one backend. Admin DSN is built once at startup from `POSTGRES_USER` + `POSTGRES_PASSWORD` + `PGPORT`.

## Conventions

- Test naming: `TestType_BehaviourBeingPinned`. Bodies 3–5 lines; setup in helpers.
- Hook signature: `func(*startup.Info) (cleanup func(), err error)`.
- Forker is stateless; every call takes `adminDSN` as the first arg.
- Unit tests use `net.Pipe + pgproto3`; integration tests use `testcontainers-go`.
- No emojis. Comments say why, not what.

## Commands

```bash
go test -short ./...            # fast (~30s, no image build)
go test -timeout=15m ./...      # full incl. e2e
docker build -t petri:postgres .
```

## Non-obvious

- `CREATE DATABASE … TEMPLATE …` needs zero active connections to the template. Petri's design holds this: clients never reach the template.
- `Drop` evicts straggler sessions before `DROP DATABASE` to win the race with Postgres's per-session cleanup.
- SSL/GSS is answered with `N` so the rest is plaintext.
- Each TCP connection on `:5433` is its own fork. Pools with `max > 1` land on different forks; tests must hold a single connection.
