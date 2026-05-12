import { buildApp } from './app';
import { newDB } from './db/connection';

// Production-style entrypoint: PGPORT defaults to 5432 (passthrough) so
// the server talks to the real DB. Tests skip this and build their own
// app inside withTestApp (see test/helpers.ts) with PGPORT=5433.
const db = newDB();
const app = buildApp(db);

const port = Number(process.env.PORT ?? 3000);
app.listen(port, () => {
  // eslint-disable-next-line no-console
  console.log(`listening on :${port}`);
});
