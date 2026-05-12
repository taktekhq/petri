import knex, { type Knex } from 'knex';

// Single source of truth for connection details. Both runtime (newDB) and
// the knex CLI (knexfile.ts) import this so migrations, seeds, the server,
// and tests all agree on host/port/credentials. Override the port via
// PGPORT — :5432 for passthrough (app + migrations), :5433 for fork-per-
// connection (tests). The jest script sets PGPORT=5433 directly.
export function dbConfig() {
  return {
    host: process.env.PGHOST ?? 'postgres',
    port: Number(process.env.PGPORT ?? 5432),
    user: 'appuser',
    password: 'apppass',
    database: 'appdb',
  };
}

// pool max=1 is required for fork-port use: one TCP connection = one fork.
// A pool with max > 1 would split queries across multiple forks and break
// isolation.
export function newDB(): Knex {
  return knex({
    client: 'pg',
    connection: dbConfig(),
    pool: { min: 1, max: 1 },
  });
}
