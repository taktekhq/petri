import knex, { type Knex } from 'knex';

// One TCP connection per Knex instance — required for fork-port isolation.
// Pools with max > 1 would split queries across multiple forks and break
// per-test isolation.
export function newDB(opts: { host?: string; port?: number } = {}): Knex {
  return knex({
    client: 'pg',
    connection: {
      host: opts.host ?? process.env.PGHOST ?? 'postgres',
      port: opts.port ?? Number(process.env.PGPORT ?? 5432),
      user: 'appuser',
      password: 'apppass',
      database: 'appdb',
    },
    pool: { min: 1, max: 1 },
  });
}
