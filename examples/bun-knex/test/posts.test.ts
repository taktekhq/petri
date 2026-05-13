import { describe, it, expect } from 'bun:test';
import { withApp } from './helpers';

describe('posts', () => {
  it('lists seeded posts', () =>
    withApp(async (app) => {
      const body = (await (await app.request('/posts')).json()) as unknown[];
      expect(body).toHaveLength(3);
    }));

  it('cascade delete stays inside its own fork', async () => {
    await withApp(async (app) => {
      // Deleting user 1 cascades to their 2 posts.
      expect((await app.request('/users/1', { method: 'DELETE' })).status).toBe(204);
      const posts = (await (await app.request('/posts')).json()) as { user_id: number }[];
      expect(posts.every((p) => p.user_id !== 1)).toBe(true);
    });
    // Fresh fork: the cascade never happened outside this connection.
    await withApp(async (app) => {
      const posts = (await (await app.request('/posts')).json()) as { user_id: number }[];
      expect(posts.filter((p) => p.user_id === 1)).toHaveLength(2);
    });
  });
});
