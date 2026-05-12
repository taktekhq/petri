#!/bin/sh
# Boots Postgres on the loopback port, then runs petri on :5432 in front.
# Postgres credentials and init scripts are configured exactly like the
# upstream postgres image — POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB,
# and any /docker-entrypoint-initdb.d/ files all keep working.
set -e

# Boot Postgres in the background using the upstream image's entrypoint.
# Force loopback-only so nothing inside the container can reach Postgres
# directly without going through petri.
docker-entrypoint.sh "$@" -c listen_addresses=127.0.0.1 &
PG_PID=$!

# Wait until Postgres is actually accepting connections on the loopback port.
# pg_isready returns 0 when Postgres is ready, !=0 otherwise.
until pg_isready -h 127.0.0.1 -p "${PGPORT}" -U "${POSTGRES_USER:-postgres}" -q; do
    sleep 0.2
done

# Compose petri's env from the standard POSTGRES_* vars. petri itself
# listens on :5432 and forwards to loopback Postgres.
export PETRI_LISTEN_ADDR=":5432"
export PETRI_BACKEND_ADDR="127.0.0.1:${PGPORT}"
export PETRI_ADMIN_DSN="postgres://${POSTGRES_USER:-postgres}:${POSTGRES_PASSWORD:-}@127.0.0.1:${PGPORT}/postgres?sslmode=disable"

# Take Postgres down with us when we receive a stop signal.
trap 'kill -TERM "$PG_PID" 2>/dev/null || true; wait "$PG_PID" 2>/dev/null || true; exit 0' INT TERM

# Run petri in the foreground; if it exits, take Postgres with it and
# propagate petri's exit code.
petri
PETRI_EXIT=$?
kill -TERM "$PG_PID" 2>/dev/null || true
wait "$PG_PID" 2>/dev/null || true
exit "$PETRI_EXIT"
