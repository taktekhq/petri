import { Hono } from 'hono';
import type { Knex } from 'knex';

export const posts = (db: Knex) => {
  const r = new Hono();

  r.get('/', async (c) =>
    c.json(await db('posts').select('*').orderBy('id')));

  return r;
};
