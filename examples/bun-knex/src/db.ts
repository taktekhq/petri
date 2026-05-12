import knex, { type Knex } from 'knex';

// Single source of truth for the connection. PGPORT picks the petri port:
// :5432 passthrough (server, migrate) or :5433 fork-per-connection (tests).
// pool max=1 is required on the fork port — one TCP connection = one fork.
export const newDB = (): Knex =>
  knex({
    client: 'pg',
    connection: {
      host: process.env.PGHOST ?? 'postgres',
      port: Number(process.env.PGPORT ?? 5432),
      user: 'appuser',
      password: 'apppass',
      database: 'appdb',
    },
    pool: { min: 1, max: 1 },
  });
