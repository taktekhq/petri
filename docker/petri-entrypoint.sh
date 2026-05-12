#!/bin/sh
# Boots Postgres on the loopback port, then runs petri on :5432 in front.
# petri reads POSTGRES_PASSWORD and PGPORT directly from the env — exactly the
# vars the upstream postgres image already documents — so this script only
# orchestrates lifecycle and does no env translation.
set -e

# Boot Postgres in the background using the upstream image's entrypoint.
# Force loopback-only so nothing inside the container can reach Postgres
# directly without going through petri.
docker-entrypoint.sh "$@" -c listen_addresses=127.0.0.1 &
PG_PID=$!

# Wait until Postgres is actually accepting connections on the loopback port.
# Passing -d avoids the harmless "database appuser does not exist" log line
# that pg_isready otherwise produces when POSTGRES_USER != "postgres".
until pg_isready \
        -h 127.0.0.1 \
        -p "${PGPORT}" \
        -U "${POSTGRES_USER:-postgres}" \
        -d "${POSTGRES_DB:-postgres}" -q; do
    sleep 0.2
done

# Take Postgres down with us when we receive a stop signal.
trap 'kill -TERM "$PG_PID" 2>/dev/null || true; wait "$PG_PID" 2>/dev/null || true; exit 0' INT TERM

# Run petri in the foreground; if it exits, take Postgres with it and
# propagate petri's exit code.
petri
PETRI_EXIT=$?
kill -TERM "$PG_PID" 2>/dev/null || true
wait "$PG_PID" 2>/dev/null || true
exit "$PETRI_EXIT"
