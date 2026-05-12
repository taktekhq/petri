import { describe, it, expect } from 'bun:test';
import { withApp, send } from './helpers';

describe('stores', () => {
  it('lists seeded stores', () =>
    withApp(async (app) => {
      expect(await (await app.request('/stores')).json()).toMatchObject([
        { id: 1, slug: 'corner-shop' },
        { id: 2, slug: 'big-box' },
      ]);
    }));

  it('reads one by id', () =>
    withApp(async (app) => {
      expect(await (await app.request('/stores/1')).json()).toMatchObject({ name: 'Corner Shop' });
    }));

  it('creates a store', () =>
    withApp(async (app) => {
      const res = await send(app, '/stores', 'POST', { name: 'Pop-up', slug: 'pop-up' });
      expect(res.status).toBe(201);
      expect(await res.json()).toMatchObject({ slug: 'pop-up' });
    }));

  it('rejects creation with missing fields', () =>
    withApp(async (app) => {
      const res = await send(app, '/stores', 'POST', { name: 'Half' });
      expect(res.status).toBe(400);
    }));

  it('patches a store', () =>
    withApp(async (app) => {
      const res = await send(app, '/stores/1', 'PATCH', { name: 'Corner Shop II' });
      expect(await res.json()).toMatchObject({ id: 1, name: 'Corner Shop II' });
    }));

  it('cascades to products on delete', () =>
    withApp(async (app) => {
      expect((await app.request('/stores/1', { method: 'DELETE' })).status).toBe(204);
      const products = (await (await app.request('/products')).json()) as { store_id: number }[];
      expect(products.every((p) => p.store_id !== 1)).toBe(true);
    }));
});
