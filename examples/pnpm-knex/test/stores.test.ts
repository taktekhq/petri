import request from 'supertest';
import { withTestApp } from './helpers';

describe('stores CRUD', () => {
  it('lists the seeded stores', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/stores');
      expect(res.status).toBe(200);
      expect(res.body.map((s: { slug: string }) => s.slug)).toEqual([
        'corner-shop',
        'big-box',
      ]);
    });
  });

  it('reads one by id', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/stores/1');
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, name: 'Corner Shop' });
    });
  });

  it('creates a store', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app)
        .post('/stores')
        .send({ name: 'Pop-up', slug: 'pop-up' });
      expect(res.status).toBe(201);
      expect(res.body).toMatchObject({ slug: 'pop-up' });
    });
  });

  it('rejects creation with missing fields', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).post('/stores').send({ name: 'Half' });
      expect(res.status).toBe(400);
    });
  });

  it('patches a store', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).patch('/stores/1').send({ name: 'Corner Shop II' });
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, name: 'Corner Shop II' });
    });
  });

  it('deletes a store and cascades to its products', async () => {
    await withTestApp(async ({ db, app }) => {
      expect((await request(app).delete('/stores/1')).status).toBe(204);
      const remainingProducts = await db('products').where({ store_id: 1 });
      expect(remainingProducts).toHaveLength(0);
    });
  });
});
