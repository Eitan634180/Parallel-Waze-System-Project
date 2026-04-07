import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from 'playwright/test';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const serverPort = process.env.END2END_SERVER_PORT || '8080';
const clientPort = process.env.END2END_CLIENT_PORT || '3000';

export default defineConfig({
  testDir: './end2end',
  timeout: 90_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${clientPort}`,
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'node scripts/start-navigation-server.mjs',
      cwd: __dirname,
      url: `http://127.0.0.1:${serverPort}/system/info`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
    },
    {
      command: 'node scripts/serve-client.mjs',
      cwd: __dirname,
      url: `http://127.0.0.1:${clientPort}/navigation.html`,
      reuseExistingServer: !process.env.CI,
      timeout: 30_000,
    },
  ],
});
