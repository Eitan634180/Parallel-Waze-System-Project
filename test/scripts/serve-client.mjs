import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

import { startStaticServer } from '../performance/loadbot/static-server.mjs';
import { testConfig } from '../config/test-config.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..', '..');
const clientDir = path.join(root, 'client');

const server = await startStaticServer(clientDir, testConfig.client.host, testConfig.client.port);
console.log(`client: serving ${clientDir} on ${testConfig.client.url}`);

let closing = false;

async function shutdown() {
  if (closing) {
    return;
  }
  closing = true;
  await new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}

for (const eventName of ['SIGINT', 'SIGTERM']) {
  process.on(eventName, async () => {
    await shutdown();
    process.exit(0);
  });
}
