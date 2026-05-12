import { Router, type Request, type Response } from 'express';
import type { Knex } from 'knex';
import { productQueries } from '../db/queries/products';

export function productsRouter(db: Knex): Router {
  const r = Router();

  r.get('/', async (_req: Request, res: Response) => {
    res.json(await productQueries.list(db));
  });

  r.get('/:id', async (req: Request, res: Response) => {
    const row = await productQueries.get(db, Number(req.params.id));
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.post('/', async (req: Request, res: Response) => {
    const { store_id, name, price_cents } = req.body ?? {};
    if (store_id === undefined || !name || price_cents === undefined) {
      return res.status(400).json({ error: 'store_id, name, price_cents required' });
    }
    const row = await productQueries.create(db, { store_id, name, price_cents });
    res.status(201).json(row);
  });

  r.patch('/:id', async (req: Request, res: Response) => {
    const row = await productQueries.update(db, Number(req.params.id), req.body ?? {});
    if (!row) return res.status(404).json({ error: 'not found' });
    res.json(row);
  });

  r.delete('/:id', async (req: Request, res: Response) => {
    const removed = await productQueries.remove(db, Number(req.params.id));
    if (!removed) return res.status(404).json({ error: 'not found' });
    res.status(204).send();
  });

  return r;
}
