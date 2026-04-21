import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig } from 'playwright/test';
import { testConfig } from './config/test-config.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export default defineConfig({
  testDir: './end2end',
  timeout: testConfig.e2e.timeoutMs,
  expect: {
    timeout: testConfig.e2e.expectTimeoutMs,
  },
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: testConfig.client.url,
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'node scripts/start-navigation-server.mjs',
      cwd: __dirname,
      url: `${testConfig.server.url}/system/info`,
      reuseExistingServer: !process.env.CI,
      timeout: testConfig.e2e.serverReadyTimeoutMs,
    },
    {
      command: 'node scripts/serve-client.mjs',
      cwd: __dirname,
      url: testConfig.client.entryURL,
      reuseExistingServer: !process.env.CI,
      timeout: testConfig.e2e.clientReadyTimeoutMs,
    },
  ],
});
