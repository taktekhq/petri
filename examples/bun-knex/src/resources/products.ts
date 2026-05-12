import { crud } from '../crud';
export const products = crud('products', ['store_id', 'name', 'price_cents']);
