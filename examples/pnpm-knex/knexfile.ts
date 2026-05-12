import type { Knex } from 'knex';

const host = process.env.PGHOST ?? 'postgres';
const port = Number(process.env.PGPORT ?? 5432);

// Migrations and seeds always go through the passthrough port (5432) so
// they land on the real template DB. Every fork on :5433 inherits the
// schema and seed data.
const config: { [env: string]: Knex.Config } = {
  development: {
    client: 'pg',
    connection: {
      host,
      port,
      user: 'appuser',
      password: 'apppass',
      database: 'appdb',
    },
    migrations: { directory: './src/db/migrations', extension: 'ts' },
    seeds: { directory: './src/db/seeds', extension: 'ts' },
  },
};

export default config;
module.exports = config;
