import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const clientRootDir = path.resolve(__dirname, '..');
const defaultOutputPath = path.join(clientRootDir, 'public', 'runtime-config.js');

function readRequiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} must be set to generate the client runtime config.`);
  }
  return value;
}

function escapeForJavaScript(value) {
  return JSON.stringify(String(value));
}

export function buildRuntimeConfigSource() {
  const serverUrl = readRequiredEnv('NAV_CLIENT_SERVER_URL');
  return [
    'window.__NAV_RUNTIME_CONFIG__ = Object.freeze({',
    `  serverUrl: ${escapeForJavaScript(serverUrl)},`,
    '});',
    '',
  ].join('\n');
}

export function writeRuntimeConfig(outputPath = defaultOutputPath) {
  mkdirSync(path.dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, buildRuntimeConfigSource(), 'utf8');
  return outputPath;
}

if (process.argv[1] === __filename) {
  writeRuntimeConfig(process.argv[2] ? path.resolve(process.argv[2]) : defaultOutputPath);
}
