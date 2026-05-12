import knex, { type Knex } from 'knex';

// Connects through the petri service over the compose network. PGPORT
// picks the petri port: :5432 passthrough (server, migrate), :5433
// fork-per-connection (tests — set by the `test` script). The rest of
// the env vars are libpq's standard names and fall back to the postgres
// image's defaults; trust auth (set on the compose service) means no
// password is needed.
export const newDB = (): Knex =>
  knex({
    client: 'pg',
    connection: {
      host: process.env.PGHOST ?? 'postgres',
      port: Number(process.env.PGPORT ?? 5432),
      user: process.env.PGUSER ?? 'postgres',
      database: process.env.PGDATABASE ?? 'postgres',
    },
    pool: { min: 1, max: 1 },
  });
