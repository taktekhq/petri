import { buildApp } from './app';
import { newDB } from './db';

// PGPORT defaults to 5432 here (passthrough). Tests override to 5433 via
// the `test` script — they don't go through this file.
export default {
  port: Number(process.env.PORT ?? 3000),
  fetch: buildApp(newDB()).fetch,
};
