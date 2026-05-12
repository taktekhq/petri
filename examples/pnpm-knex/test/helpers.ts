import type { Knex } from 'knex';
import { newDB } from '../src/db/connection';
import { buildApp } from '../src/app';

// Every call to newTestDB() opens ONE TCP connection to the fork port (5433),
// which lands on a fresh CREATE DATABASE … TEMPLATE appdb. Closing the
// connection drops the fork. Each test gets its own fork — files run in
// parallel across Jest workers, and tests inside a file are sequential
// against the same fork.
export function newTestDB(): Knex {
  return newDB({ port: 5433 });
}

export type TestCtx = {
  db: Knex;
  app: ReturnType<typeof buildApp>;
};

export async function withTestApp(fn: (ctx: TestCtx) => Promise<void>): Promise<void> {
  const db = newTestDB();
  try {
    await fn({ db, app: buildApp(db) });
  } finally {
    await db.destroy();
  }
}
