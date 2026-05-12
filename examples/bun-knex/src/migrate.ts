import { newDB } from './db';

// Idempotent reset: drops everything, recreates the schema, seeds fixtures.
// Runs on PGPORT=5432 (passthrough) so the writes land on the real
// template DB; every fork on :5433 inherits them.
const db = newDB();

await db.schema.dropTableIfExists('products');
await db.schema.dropTableIfExists('stores');
await db.schema.dropTableIfExists('users');

await db.schema.createTable('users', (t) => {
  t.increments('id');
  t.string('email').notNullable().unique();
  t.string('name').notNullable();
});

await db.schema.createTable('stores', (t) => {
  t.increments('id');
  t.string('name').notNullable();
  t.string('slug').notNullable().unique();
});

await db.schema.createTable('products', (t) => {
  t.increments('id');
  t.integer('store_id').notNullable().references('id').inTable('stores').onDelete('CASCADE');
  t.string('name').notNullable();
  t.integer('price_cents').notNullable();
});

await db('users').insert([
  { email: 'alice@example.com', name: 'Alice' },
  { email: 'bob@example.com', name: 'Bob' },
]);

await db('stores').insert([
  { name: 'Corner Shop', slug: 'corner-shop' },
  { name: 'Big Box', slug: 'big-box' },
]);

await db('products').insert([
  { store_id: 1, name: 'Apple', price_cents: 50 },
  { store_id: 1, name: 'Bread', price_cents: 300 },
  { store_id: 2, name: 'TV', price_cents: 49900 },
]);

await db.destroy();
