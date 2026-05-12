import { Router, type Request, type Response } from 'express';
import type { Knex } from 'knex';
import { storeQueries } from '../db/queries/stores';

export function storesRouter(db: Knex): Router {
  const r = Router();

  r.get('/', async (_req: Request, res: Response) => {
    res.json(await storeQueries.list(db));
  });

  r.get('/:id', async (req: Request, res: Response) => {
    const row = await storeQueries.get(db, Number(req.params.id));
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.post('/', async (req: Request, res: Response) => {
    const { name, slug } = req.body ?? {};
    if (!name || !slug) return res.status(400).json({ error: 'name and slug required' });
    const row = await storeQueries.create(db, { name, slug });
    res.status(201).json(row);
  });

  r.patch('/:id', async (req: Request, res: Response) => {
    const row = await storeQueries.update(db, Number(req.params.id), req.body ?? {});
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.delete('/:id', async (req: Request, res: Response) => {
    const removed = await storeQueries.remove(db, Number(req.params.id));
    if (!removed) return res.status(404).json({ error: 'not found' });
    res.status(204).send();
  });

  return r;
}
