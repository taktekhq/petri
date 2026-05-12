import type { Knex } from 'knex';
import { dbConfig } from './src/db/connection';

// The knex CLI loads this for migrations and seeds. Connection details
// come from src/db/connection.ts so there's exactly one place to change
// them. The CLI is invoked with PGPORT unset (defaulting to :5432
// passthrough), so migrations and seeds land on the real template DB and
// every fork on :5433 inherits them.
const config: { [env: string]: Knex.Config } = {
  development: {
    client: 'pg',
    connection: dbConfig(),
    migrations: { directory: './src/db/migrations', extension: 'ts' },
    seeds: { directory: './src/db/seeds', extension: 'ts' },
  },
};

export default config;
module.exports = config;
