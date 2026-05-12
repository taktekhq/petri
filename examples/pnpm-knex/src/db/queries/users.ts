import type { Knex } from 'knex';

export type User = {
  id: number;
  email: string;
  name: string;
  created_at: Date;
};

export type NewUser = Pick<User, 'email' | 'name'>;
export type UserPatch = Partial<NewUser>;

const table = 'users';

export const userQueries = {
  list: (db: Knex) => db<User>(table).select('*').orderBy('id'),

  get: (db: Knex, id: number) => db<User>(table).where({ id }).first(),

  create: async (db: Knex, input: NewUser): Promise<User> => {
    const [row] = await db<User>(table).insert(input).returning('*');
    return row;
  },

  update: async (db: Knex, id: number, patch: UserPatch): Promise<User | undefined> => {
    const [row] = await db<User>(table).where({ id }).update(patch).returning('*');
    return row;
  },

  remove: (db: Knex, id: number) => db<User>(table).where({ id }).delete(),
};
