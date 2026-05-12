import { buildApp } from './app';
import { newDB } from './db';

export default {
  port: Number(process.env.PORT ?? 3000),
  fetch: buildApp(newDB()).fetch,
};
