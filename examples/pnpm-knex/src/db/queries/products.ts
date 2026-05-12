import type { Knex } from 'knex';

export type Product = {
  id: number;
  store_id: number;
  name: string;
  price_cents: number;
  created_at: Date;
};

export type NewProduct = Pick<Product, 'store_id' | 'name' | 'price_cents'>;
export type ProductPatch = Partial<NewProduct>;

const table = 'products';

export const productQueries = {
  list: (db: Knex) => db<Product>(table).select('*').orderBy('id'),

  get: (db: Knex, id: number) => db<Product>(table).where({ id }).first(),

  create: async (db: Knex, input: NewProduct): Promise<Product> => {
    const [row] = await db<Product>(table).insert(input).returning('*');
    return row;
  },

  update: async (db: Knex, id: number, patch: ProductPatch): Promise<Product | undefined> => {
    const [row] = await db<Product>(table).where({ id }).update(patch).returning('*');
    return row;
  },

  remove: (db: Knex, id: number) => db<Product>(table).where({ id }).delete(),
};
