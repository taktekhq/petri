import { newDB } from '../src/db';
import { buildApp } from '../src/app';

type App = ReturnType<typeof buildApp>;

// One TCP connection per withApp call = one fresh fork; closing drops it.
// Tests inside a file run sequentially against the SAME process but each
// `it` opens its own withApp, so each test gets its own fork. Test FILES
// run in parallel across bun:test workers.
export const withApp = async <T>(fn: (app: App) => Promise<T>): Promise<T> => {
  const db = newDB();
  try {
    return await fn(buildApp(db));
  } finally {
    await db.destroy();
  }
};

// Tiny JSON-body helper. Hono's app.request() accepts the same init
// object as fetch(); this just bakes in the header + JSON.stringify.
export const send = (app: App, path: string, method: string, body?: unknown) =>
  app.request(path, {
    method,
    headers: { 'content-type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
