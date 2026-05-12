import type { Knex } from 'knex';
import { newDB } from '../src/db/connection';
import { buildApp } from '../src/app';

// The `test` script in package.json sets PGPORT=5433, so newDB() here
// opens a fork-per-connection. Each withTestApp call is one TCP connection
// = one fresh fork; closing it drops the fork.
export type TestCtx = {
  db: Knex;
  app: ReturnType<typeof buildApp>;
};

export async function withTestApp(fn: (ctx: TestCtx) => Promise<void>): Promise<void> {
  const db = newDB();
  try {
    await fn({ db, app: buildApp(db) });
  } finally {
    await db.destroy();
  }
}
