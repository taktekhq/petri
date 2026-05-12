import { describe, it, expect } from 'bun:test';
import { withApp, send } from './helpers';

describe('products', () => {
  it('lists seeded products', () =>
    withApp(async (app) => {
      const body = (await (await app.request('/products')).json()) as unknown[];
      expect(body).toHaveLength(3);
    }));

  it('reads one by id', () =>
    withApp(async (app) => {
      expect(await (await app.request('/products/1')).json()).toMatchObject({
        name: 'Apple',
        price_cents: 50,
      });
    }));

  it('creates a product', () =>
    withApp(async (app) => {
      const res = await send(app, '/products', 'POST', {
        store_id: 2,
        name: 'Phone',
        price_cents: 79900,
      });
      expect(res.status).toBe(201);
      expect(await res.json()).toMatchObject({ store_id: 2, name: 'Phone' });
    }));

  it('rejects creation with missing fields', () =>
    withApp(async (app) => {
      const res = await send(app, '/products', 'POST', { name: 'Orphan' });
      expect(res.status).toBe(400);
    }));

  it('patches a product', () =>
    withApp(async (app) => {
      const res = await send(app, '/products/1', 'PATCH', { price_cents: 99 });
      expect(await res.json()).toMatchObject({ id: 1, price_cents: 99 });
    }));

  it('deletes only inside its own fork', async () => {
    await withApp(async (app) => {
      expect((await app.request('/products/3', { method: 'DELETE' })).status).toBe(204);
      expect((await app.request('/products/3')).status).toBe(404);
    });
    await withApp(async (app) => {
      expect((await app.request('/products/3')).status).toBe(200);
    });
  });
});
