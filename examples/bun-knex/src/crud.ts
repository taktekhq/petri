import { Hono } from 'hono';
import type { Knex } from 'knex';

// Single CRUD factory for every resource. Each resource module passes a
// table name and the set of writable columns; we whitelist the body
// against that set and require all of them on POST. List/get/patch/delete
// are identical across resources, so they live here once.
const pick = (body: Record<string, unknown>, fields: readonly string[]) =>
  Object.fromEntries(fields.filter((f) => body[f] !== undefined).map((f) => [f, body[f]]));

export const crud = (table: string, fields: readonly string[]) => (db: Knex) => {
  const r = new Hono();

  r.get('/', async (c) => c.json(await db(table).select('*').orderBy('id')));

  r.get('/:id', async (c) => {
    const row = await db(table).where({ id: c.req.param('id') }).first();
    return row ? c.json(row) : c.json({ error: 'not found' }, 404);
  });

  r.post('/', async (c) => {
    const body = (await c.req.json()) as Record<string, unknown>;
    const missing = fields.filter((f) => body[f] === undefined);
    if (missing.length) return c.json({ error: `${missing.join(', ')} required` }, 400);
    const [row] = await db(table).insert(pick(body, fields)).returning('*');
    return c.json(row, 201);
  });

  r.patch('/:id', async (c) => {
    const body = (await c.req.json()) as Record<string, unknown>;
    const [row] = await db(table)
      .where({ id: c.req.param('id') })
      .update(pick(body, fields))
      .returning('*');
    return row ? c.json(row) : c.json({ error: 'not found' }, 404);
  });

  r.delete('/:id', async (c) => {
    const n = await db(table).where({ id: c.req.param('id') }).delete();
    return n ? new Response(null, { status: 204 }) : c.json({ error: 'not found' }, 404);
  });

  return r;
};
