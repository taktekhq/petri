import { Hono } from 'hono';
import type { Knex } from 'knex';

export const users = (db: Knex) => {
  const r = new Hono();

  r.get('/', async (c) =>
    c.json(await db('users').select('*').orderBy('id')));

  r.get('/:id', async (c) => {
    const row = await db('users').where({ id: c.req.param('id') }).first();
    return row ? c.json(row) : c.json({ error: 'not found' }, 404);
  });

  r.delete('/:id', async (c) => {
    const n = await db('users').where({ id: c.req.param('id') }).delete();
    return n ? new Response(null, { status: 204 }) : c.json({ error: 'not found' }, 404);
  });

  return r;
};
