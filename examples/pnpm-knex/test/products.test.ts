import request from 'supertest';
import { withTestApp } from './helpers';

describe('products CRUD', () => {
  it('lists the seeded products', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/products');
      expect(res.status).toBe(200);
      expect(res.body).toHaveLength(3);
    });
  });

  it('reads one by id', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/products/1');
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, name: 'Apple', price_cents: 50 });
    });
  });

  it('creates a product', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app)
        .post('/products')
        .send({ store_id: 2, name: 'Phone', price_cents: 79900 });
      expect(res.status).toBe(201);
      expect(res.body).toMatchObject({ store_id: 2, name: 'Phone' });
    });
  });

  it('rejects creation with missing fields', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).post('/products').send({ name: 'Orphan' });
      expect(res.status).toBe(400);
    });
  });

  it('patches a product', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).patch('/products/1').send({ price_cents: 99 });
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, price_cents: 99 });
    });
  });

  it('deletes a product', async () => {
    await withTestApp(async ({ app }) => {
      expect((await request(app).delete('/products/3')).status).toBe(204);
      expect((await request(app).get('/products/3')).status).toBe(404);
    });
  });

  it('isolates writes across parallel-style scenarios', async () => {
    // Two separate forks within the same test file: a delete in fork A is
    // invisible in fork B. (Across files, Jest workers give the same
    // guarantee for free.)
    await withTestApp(async ({ app }) => {
      expect((await request(app).delete('/products/1')).status).toBe(204);
    });
    await withTestApp(async ({ app }) => {
      expect((await request(app).get('/products/1')).status).toBe(200);
    });
  });
});
