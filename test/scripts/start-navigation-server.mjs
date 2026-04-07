import { mkdirSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..', '..');
const serverDir = path.join(root, 'server');
const cacheDir = path.join(root, '.cache', 'playwright-go-build');
const serverPort = process.env.END2END_SERVER_PORT || '8080';
const regionDir = process.env.END2END_REGION_DIR || path.join(serverDir, 'data', 'map', 'israel-and-palestine');

mkdirSync(cacheDir, { recursive: true });

const child = spawn(
  'go',
  ['run', './cmd/server', '--addr', `127.0.0.1:${serverPort}`, '--data', regionDir],
  {
    cwd: serverDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOCACHE: cacheDir,
    },
  },
);

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});

for (const eventName of ['SIGINT', 'SIGTERM']) {
  process.on(eventName, () => {
    child.kill(eventName);
  });
}
