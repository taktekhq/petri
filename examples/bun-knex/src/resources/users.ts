import { crud } from '../crud';
export const users = crud('users', ['email', 'name']);
