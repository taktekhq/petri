import type { Knex } from 'knex';

export type Store = {
  id: number;
  name: string;
  slug: string;
  created_at: Date;
};

export type NewStore = Pick<Store, 'name' | 'slug'>;
export type StorePatch = Partial<NewStore>;

const table = 'stores';

export const storeQueries = {
  list: (db: Knex) => db<Store>(table).select('*').orderBy('id'),

  get: (db: Knex, id: number) => db<Store>(table).where({ id }).first(),

  create: async (db: Knex, input: NewStore): Promise<Store> => {
    const [row] = await db<Store>(table).insert(input).returning('*');
    return row;
  },

  update: async (db: Knex, id: number, patch: StorePatch): Promise<Store | undefined> => {
    const [row] = await db<Store>(table).where({ id }).update(patch).returning('*');
    return row;
  },

  remove: (db: Knex, id: number) => db<Store>(table).where({ id }).delete(),
};
