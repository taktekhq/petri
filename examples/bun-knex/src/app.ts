import { Hono } from 'hono';
import type { Knex } from 'knex';
import { users } from './resources/users';
import { posts } from './resources/posts';

export const buildApp = (db: Knex) => {
  const app = new Hono();
  app.route('/users', users(db));
  app.route('/posts', posts(db));
  return app;
};
