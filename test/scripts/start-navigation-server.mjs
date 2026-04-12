import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { testConfig } from '../config/test-config.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..', '..');
const serverDir = path.join(root, 'server');
const cacheDir = path.join(root, '.cache', 'playwright-go-build');
const requiredGraphFiles = ['nodes.bin', 'edges.bin', 'base_adj.bin', 'cells.bin', 'boundary.bin', 'overlay_adj.bin'];

mkdirSync(cacheDir, { recursive: true });

function isReadyRegion(dir) {
  if (!dir) {
    return false;
  }
  return requiredGraphFiles.every((name) => existsSync(path.join(dir, name)));
}

function resolveRegionDir() {
  if (isReadyRegion(testConfig.regionDir)) {
    return testConfig.regionDir;
  }

  if (!testConfig.regionDir) {
    throw new Error(`TEST_REGION_DIR was not set in ${testConfig.envFilePath}.`);
  }

  for (const name of requiredGraphFiles) {
    const expectedPath = path.join(testConfig.regionDir, name);
    if (!existsSync(expectedPath)) {
      throw new Error(`End-to-end region file not found: ${expectedPath}`);
    }
  }

  return testConfig.regionDir;
}

const regionDir = resolveRegionDir();

const child = spawn(
  'go',
  ['run', './cmd/server', '--addr', testConfig.server.listenAddr, '--data', regionDir, '--routing-mode', testConfig.server.routingMode],
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
