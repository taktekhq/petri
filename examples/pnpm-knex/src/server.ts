import { buildApp } from './app';
import { newDB } from './db/connection';

// Production-style entrypoint: connects to the passthrough port (5432) and
// serves CRUD over the real DB. Tests skip this and build their own app
// against a fork-port connection (see test/helpers.ts).
const db = newDB({ port: 5432 });
const app = buildApp(db);

const port = Number(process.env.PORT ?? 3000);
app.listen(port, () => {
  // eslint-disable-next-line no-console
  console.log(`listening on :${port}`);
});
