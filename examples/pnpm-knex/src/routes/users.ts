import { Router, type Request, type Response } from 'express';
import type { Knex } from 'knex';
import { userQueries } from '../db/queries/users';

export function usersRouter(db: Knex): Router {
  const r = Router();

  r.get('/', async (_req: Request, res: Response) => {
    res.json(await userQueries.list(db));
  });

  r.get('/:id', async (req: Request, res: Response) => {
    const row = await userQueries.get(db, Number(req.params.id));
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.post('/', async (req: Request, res: Response) => {
    const { email, name } = req.body ?? {};
    if (!email || !name) return res.status(400).json({ error: 'email and name required' });
    const row = await userQueries.create(db, { email, name });
    res.status(201).json(row);
  });

  r.patch('/:id', async (req: Request, res: Response) => {
    const row = await userQueries.update(db, Number(req.params.id), req.body ?? {});
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.delete('/:id', async (req: Request, res: Response) => {
    const removed = await userQueries.remove(db, Number(req.params.id));
    if (!removed) return res.status(404).json({ error: 'not found' });
    res.status(204).send();
  });

  return r;
}
