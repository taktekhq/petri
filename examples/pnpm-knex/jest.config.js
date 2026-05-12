/** @type {import('jest').Config} */
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  testMatch: ['<rootDir>/test/**/*.test.ts'],
  // Jest already runs test FILES in parallel across workers; the routes are
  // split one-file-per-resource so each worker gets its own file. Within a
  // file, tests run serially against a single connection (one fork).
  maxWorkers: '50%',
  testTimeout: 30_000,
};
