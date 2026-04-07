import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

import { startStaticServer } from '../performance/loadbot/static-server.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..', '..');
const clientDir = path.join(root, 'client');
const clientPort = Number(process.env.END2END_CLIENT_PORT || '3000');

const server = await startStaticServer(clientDir, clientPort);
console.log(`client: serving ${clientDir} on http://127.0.0.1:${clientPort}`);

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
