import { describe, it, expect } from 'bun:test';
import { withApp } from './helpers';

describe('users', () => {
  it('lists seeded users', () =>
    withApp(async (app) => {
      const res = await app.request('/users');
      expect(res.status).toBe(200);
      expect(await res.json()).toMatchObject([
        { id: 1, name: 'Alice' },
        { id: 2, name: 'Bob' },
      ]);
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
