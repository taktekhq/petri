# Petri — orientation for Claude

Postgres proxy that gives each TCP client its own forked database.
Drop-in: `image: postgres` → `image: petri:postgres`.

## Layout

```
cmd/petri/        main + forkPerConnection hook
internal/proxy/   Serve loop, handleClient, bridge
internal/startup/ StartupMessage parse + replay
internal/forker/  CREATE/DROP DATABASE
docker/           entrypoint that backgrounds postgres, execs petri
e2e/              builds petri:test, drives it through real pgx
Dockerfile        golang:alpine builds petri → postgres:alpine bundles it
vendor/           gitignored; `go mod vendor` for offline builds
```

Every `*.go` has a matching `_test.go`.

## Per-connection flow (`internal/proxy/proxy.go`)

```
handleClient(client):
    startup.Read(client)            # parse, reject SSL/GSS
    OnStartup(info)                 # forks DB, mutates info.Database
    net.Dial(BackendAddr)           # then info.WriteTo(backend) replays startup
    bridge(client, backend)         # pipe until either side closes
    cleanup()                       # drops the fork
```

The hook lives in `cmd/petri/main.go:forkPerConnection`. Admin DSN is built
once at startup from `POSTGRES_USER` + `POSTGRES_PASSWORD` + `PGPORT`. Client
queries flow through the bridge with their own credentials.

## Conventions

- Test naming: `TestType_BehaviourBeingPinned`. Bodies are 3–5 lines; setup
  lives in helpers at the bottom of the file.
- Hook signature: `func(*startup.Info) (cleanup func(), err error)`. Cleanup
  may be nil; if non-nil it always runs after `bridge`.
- Forker is stateless; every call takes `adminDSN` as the first arg.
- Unit tests use `net.Pipe + pgproto3`; integration tests use
  `testcontainers-go`. Each top-level test spins its own container.
- No emojis. Comments say why, not what.

## Commands

```bash
go test -short ./...            # fast (~30s, no image build)
go test -timeout=15m ./...      # full incl. e2e
go test -run TestImage_Parallel ./e2e/...
docker build -t petri:postgres .
```

## Non-obvious

- `CREATE DATABASE … TEMPLATE …` needs zero active connections to the
  template. Petri's design holds this: clients never reach the template.
- `Drop` evicts straggler sessions before `DROP DATABASE` to win the race
  with Postgres's per-session cleanup.
- SSL/GSS handshakes are answered with `N` so the rest is plaintext.
- Each TCP connection is its own fork. Pools with `max > 1` will land
  on different forks; tests must hold a single connection.
- In restricted networks, pre-run `go mod vendor` — `go build` picks it up
  automatically (Go ≥ 1.14).
