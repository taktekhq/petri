import { describe, it, expect } from 'bun:test';
import { withApp, send } from './helpers';

describe('users', () => {
  it('lists seeded users', () =>
    withApp(async (app) => {
      const res = await app.request('/users');
      expect(res.status).toBe(200);
      expect(await res.json()).toMatchObject([
        { id: 1, email: 'alice@example.com', name: 'Alice' },
        { id: 2, email: 'bob@example.com', name: 'Bob' },
      ]);
    }));

  it('reads one by id', () =>
    withApp(async (app) => {
      expect(await (await app.request('/users/1')).json()).toMatchObject({ name: 'Alice' });
    }));

  it('returns 404 for unknown id', () =>
    withApp(async (app) => {
      expect((await app.request('/users/9999')).status).toBe(404);
    }));

  it('creates a user', () =>
    withApp(async (app) => {
      const res = await send(app, '/users', 'POST', { email: 'carol@example.com', name: 'Carol' });
      expect(res.status).toBe(201);
      expect(await res.json()).toMatchObject({ email: 'carol@example.com', name: 'Carol' });
    }));

  it('rejects creation with missing fields', () =>
    withApp(async (app) => {
      const res = await send(app, '/users', 'POST', { email: 'nobody@example.com' });
      expect(res.status).toBe(400);
    }));

  it('patches a user', () =>
    withApp(async (app) => {
      const res = await send(app, '/users/1', 'PATCH', { name: 'Alice II' });
      expect(await res.json()).toMatchObject({ id: 1, name: 'Alice II' });
    }));

  it('deletes only inside its own fork', async () => {
    await withApp(async (app) => {
      expect((await app.request('/users/2', { method: 'DELETE' })).status).toBe(204);
      expect((await app.request('/users/2')).status).toBe(404);
    });
    // Fresh fork still sees Bob — proves the delete didn't leak.
    await withApp(async (app) => {
      expect((await app.request('/users/2')).status).toBe(200);
    });
  });
});
