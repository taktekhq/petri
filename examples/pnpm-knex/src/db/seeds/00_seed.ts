import type { Knex } from 'knex';

// Seeded data lands on the template DB (via :5432). Every fork on :5433
// inherits these rows, so tests can rely on them as fixtures.
export async function seed(knex: Knex): Promise<void> {
  await knex('products').del();
  await knex('stores').del();
  await knex('users').del();

  await knex('users').insert([
    { id: 1, email: 'alice@example.com', name: 'Alice' },
    { id: 2, email: 'bob@example.com', name: 'Bob' },
  ]);
  await knex.raw("SELECT setval('users_id_seq', (SELECT max(id) FROM users))");

  await knex('stores').insert([
    { id: 1, name: 'Corner Shop', slug: 'corner-shop' },
    { id: 2, name: 'Big Box', slug: 'big-box' },
  ]);
  await knex.raw("SELECT setval('stores_id_seq', (SELECT max(id) FROM stores))");

  await knex('products').insert([
    { id: 1, store_id: 1, name: 'Apple', price_cents: 50 },
    { id: 2, store_id: 1, name: 'Bread', price_cents: 300 },
    { id: 3, store_id: 2, name: 'TV', price_cents: 49900 },
  ]);
  await knex.raw("SELECT setval('products_id_seq', (SELECT max(id) FROM products))");
}
