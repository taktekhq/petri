import knex, { type Knex } from 'knex';

// Connects through the petri service over the compose network. PGPORT
// picks the petri port: :5432 passthrough (server, migrate), :5433
// fork-per-connection (tests — set by the `test` script). The rest are
// libpq's standard env names; the fallbacks match the credentials the
// compose file sets on the postgres service.
export const newDB = (): Knex =>
  knex({
    client: 'pg',
    connection: {
      host: process.env.PGHOST ?? 'postgres',
      port: Number(process.env.PGPORT ?? 5432),
      user: process.env.PGUSER ?? 'postgres',
      password: process.env.PGPASSWORD ?? 'postgres',
      database: process.env.PGDATABASE ?? 'postgres',
    },
    pool: { min: 1, max: 1 },
  });
