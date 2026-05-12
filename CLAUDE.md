# Petri — project orientation for Claude

A Postgres proxy that gives each TCP client its own forked database. Drop-in
replacement for `image: postgres` in docker-compose: `image: petri:postgres`.

## File layout

```
.
├── cmd/petri/             — main + run + forkPerConnection hook
├── internal/
│   ├── proxy/             — Proxy.Serve accept loop + handleClient + bridge
│   ├── startup/           — Postgres StartupMessage parsing + replay
│   └── forker/            — stateless Fork / Drop wrapping CREATE/DROP DATABASE
├── docker/
│   └── petri-entrypoint.sh — backgrounds postgres, execs petri
├── e2e/
│   ├── image_test.go      — builds petri:test, smoke + isolation + drop
│   └── parallel_test.go   — realistic seeded parallel-worker scenario
├── Dockerfile             — golang:alpine builds petri → postgres:alpine bundles it
└── vendor/                — gitignored; regenerate with `go mod vendor` if needed
```

Each `*.go` file has a matching `_test.go` next to it.

## Per-connection flow inside `internal/proxy/proxy.go`

```
handleClient(client):
    info, err := startup.Read(client)           // parse StartupMessage, reject SSL/GSS
    cleanup, err := p.OnStartup(info)           // forks DB, mutates info.Database
    backend, err := p.dialBackend(info)         // dial + replay (rewritten) startup
    bridge(client, backend)                     // pipe bytes both ways until close
    cleanup()                                    // drops the fork
```

The hook lives in `cmd/petri/main.go` (`forkPerConnection`). Its admin DSN is
built once at startup from `POSTGRES_USER` + `POSTGRES_PASSWORD` + `PGPORT`.
The client's own queries flow through the bridge with their own credentials,
so read-only roles / RLS / etc. are preserved.

## Conventions

- **Test naming**: `TestType_BehaviourBeingPinned`. Each test body is 3–5 lines
  of intent; setup lives in named helpers at the bottom of the file.
- **Hook signature**: `func(*startup.Info) (cleanup func(), err error)`. Cleanup
  may be `nil`; if non-nil it runs unconditionally after `bridge` returns.
- **Forker is stateless**: every call takes `adminDSN` as first arg.
- **Unit tests use net.Pipe + pgproto3**; integration tests use `testcontainers-go`.
  Top-level test bodies aren't `t.Parallel()` (each spins a container), but
  subtests within a single container are.
- **No emojis. No comments that restate the code.** Comments explain WHY a
  choice was made, not WHAT the code does.

## Build, test, run

```bash
# everything fast (skips e2e image builds)
go test -short ./...

# everything including e2e (rebuilds petri:test once per test)
go test -timeout=15m ./...

# just one e2e test
go test -v -run TestImage_ParallelWorkersSeeIndependentForks ./e2e/...

# build the bundled image manually
docker build -t petri:postgres .
```

Docker daemon must be running. The e2e suite (`./e2e/...`) builds the image via
testcontainers — first run is ~30s, subsequent runs reuse the cached image.

## Non-obvious constraints

- **Postgres `CREATE DATABASE … TEMPLATE …` requires zero active connections to
  the template.** Petri's design holds this naturally: clients never reach the
  template directly — petri forks first, then dials the backend on the fork.
- **`Drop` terminates straggler connections** to the fork before `DROP DATABASE`,
  to avoid losing the race against Postgres's per-session cleanup.
- **SSL/GSS handshakes are rejected with `N`** so the rest of the conversation
  is plaintext that petri can inspect/rewrite. Petri does not speak TLS to
  clients today.
- **Each TCP connection is its own fork.** Connection pools (Knex, database/sql
  with `MaxOpenConns > 1`) will land on multiple forks and *not* share state.
  Tests should hold a single connection per logical test case — see README.

## Vendor / build TLS

The repo intentionally does **not** commit `vendor/`. The Dockerfile uses
plain `go build`, which:
- auto-uses `./vendor/` if present (handy for restricted networks: run
  `go mod vendor` once, then offline builds work)
- otherwise fetches from the configured module proxy

In sandboxed dev environments where the container can't TLS-verify
`proxy.golang.org`, pre-run `go mod vendor` on the host before `docker build`
or `go test ./e2e/...`.

## Git workflow

- Active branch: `claude/isolated-test-databases-38sFO`.
- Phases history is in commit messages; each phase ships as one or two commits.
- Never amend already-pushed commits; create a new commit.

## When extending

- New hook behaviour → add to `forkPerConnection` in `cmd/petri/main.go`, and
  the test in `cmd/petri/main_test.go` (fake forker).
- New SQL admin op → add to `internal/forker/forker.go`, test against real
  Postgres in `internal/forker/forker_test.go`.
- New protocol-level behaviour → add to `internal/startup/startup.go`, test
  with `net.Pipe + pgproto3` in `internal/startup/startup_test.go`.
- New user-facing artifact → add to `e2e/`, build the petri image and exercise
  it via testcontainers.
