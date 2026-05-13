import { newDB } from './db';

// Idempotent reset: drops everything, recreates the schema, seeds fixtures.
// Runs on PGPORT=5432 (passthrough) so the writes land on the real
// template DB; every fork on :5433 inherits them.
const db = newDB();

await db.schema.dropTableIfExists('posts');
await db.schema.dropTableIfExists('users');

await db.schema.createTable('users', (t) => {
  t.increments('id');
  t.string('email').notNullable().unique();
  t.string('name').notNullable();
});

await db.schema.createTable('posts', (t) => {
  t.increments('id');
  t.integer('user_id').notNullable().references('id').inTable('users').onDelete('CASCADE');
  t.string('title').notNullable();
});

await db('users').insert([
  { email: 'alice@example.com', name: 'Alice' },
  { email: 'bob@example.com', name: 'Bob' },
]);

await db('posts').insert([
  { user_id: 1, title: 'Hello from Alice' },
  { user_id: 1, title: 'Alice again' },
  { user_id: 2, title: 'Hello from Bob' },
]);

await db.destroy();
