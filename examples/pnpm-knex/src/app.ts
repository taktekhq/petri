import express, { type Express } from 'express';
import type { Knex } from 'knex';
import { usersRouter } from './routes/users';
import { storesRouter } from './routes/stores';
import { productsRouter } from './routes/products';

export function buildApp(db: Knex): Express {
  const app = express();
  app.use(express.json());
  app.use('/users', usersRouter(db));
  app.use('/stores', storesRouter(db));
  app.use('/products', productsRouter(db));
  return app;
}
