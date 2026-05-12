import request from 'supertest';
import { withTestApp } from './helpers';

describe('users CRUD', () => {
  it('lists the seeded users', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/users');
      expect(res.status).toBe(200);
      expect(res.body.map((u: { email: string }) => u.email)).toEqual([
        'alice@example.com',
        'bob@example.com',
      ]);
    });
  });

  it('reads one by id', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/users/1');
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, name: 'Alice' });
    });
  });

  it('returns 404 for an unknown id', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).get('/users/9999');
      expect(res.status).toBe(404);
    });
  });

  it('creates a user', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app)
        .post('/users')
        .send({ email: 'carol@example.com', name: 'Carol' });
      expect(res.status).toBe(201);
      expect(res.body).toMatchObject({ email: 'carol@example.com', name: 'Carol' });
      expect(typeof res.body.id).toBe('number');
    });
  });

  it('rejects creation with missing fields', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).post('/users').send({ email: 'nobody@example.com' });
      expect(res.status).toBe(400);
    });
  });

  it('patches a user', async () => {
    await withTestApp(async ({ app }) => {
      const res = await request(app).patch('/users/1').send({ name: 'Alice II' });
      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({ id: 1, name: 'Alice II' });
    });
  });

  it('deletes a user and isolates the deletion to this fork', async () => {
    await withTestApp(async ({ app }) => {
      expect((await request(app).delete('/users/2')).status).toBe(204);
      expect((await request(app).get('/users/2')).status).toBe(404);
    });
    // A fresh fork still sees Bob — proves the previous test's delete
    // didn't leak across forks.
    await withTestApp(async ({ app }) => {
      expect((await request(app).get('/users/2')).status).toBe(200);
    });
  });
});
