import { Hono } from 'hono';
import type { Knex } from 'knex';
import { users } from './resources/users';
import { stores } from './resources/stores';
import { products } from './resources/products';

export const buildApp = (db: Knex) => {
  const app = new Hono();
  app.route('/users', users(db));
  app.route('/stores', stores(db));
  app.route('/products', products(db));
  return app;
};
