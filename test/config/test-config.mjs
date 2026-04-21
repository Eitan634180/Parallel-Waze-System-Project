import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const testDir = path.resolve(__dirname, '..');
const rootDir = path.resolve(testDir, '..');
const envFilePath = path.join(rootDir, 'project.env.test');

loadEnvFile(envFilePath);

function loadEnvFile(filePath) {
  if (!existsSync(filePath)) {
    return;
  }

  const contents = readFileSync(filePath, 'utf8');
  for (const rawLine of contents.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) {
      continue;
    }

    const separatorIndex = line.indexOf('=');
    if (separatorIndex <= 0) {
      continue;
    }

    const key = line.slice(0, separatorIndex).trim();
    const value = line.slice(separatorIndex + 1).trim();
    if (!key || Object.prototype.hasOwnProperty.call(process.env, key)) {
      continue;
    }
    process.env[key] = value;
  }
}

function readString(name, fallback = '') {
  const value = process.env[name];
  if (value == null) {
    return fallback;
  }

  const trimmed = value.trim();
  return trimmed === '' ? fallback : trimmed;
}

function isAbsolutePath(value) {
  return path.isAbsolute(value) || /^[A-Za-z]:/.test(value);
}

function resolveMapRelativePath(name, fallback = '') {
  const raw = readString(name, fallback);
  if (raw === '') {
    return fallback;
  }
  if (isAbsolutePath(raw)) {
    throw new Error(`${name} in ${envFilePath} must be relative to server/data/map.`);
  }
  return path.join(rootDir, 'server', 'data', 'map', raw);
}

function requireString(name) {
  const value = readString(name, '');
  if (value === '') {
    throw new Error(`${name} must be set in ${envFilePath}.`);
  }
  return value;
}

function requireInteger(name, minimum = null) {
  const raw = requireString(name);
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed)) {
    throw new Error(`${name} in ${envFilePath} must be an integer.`);
  }
  if (minimum != null && parsed < minimum) {
    throw new Error(`${name} in ${envFilePath} must be at least ${minimum}.`);
  }
  return parsed;
}

function requireFloat(name, minimum = null, maximum = null) {
  const raw = requireString(name);
  const parsed = Number.parseFloat(raw);
  if (Number.isNaN(parsed)) {
    throw new Error(`${name} in ${envFilePath} must be a number.`);
  }
  if (minimum != null && parsed < minimum) {
    throw new Error(`${name} in ${envFilePath} must be at least ${minimum}.`);
  }
  if (maximum != null && parsed > maximum) {
    throw new Error(`${name} in ${envFilePath} must be at most ${maximum}.`);
  }
  return parsed;
}

function requireBoolean(name) {
  const raw = requireString(name);
  switch (raw.toLowerCase()) {
    case '1':
    case 'true':
    case 'yes':
    case 'on':
      return true;
    case '0':
    case 'false':
    case 'no':
    case 'off':
      return false;
    default:
      throw new Error(`${name} in ${envFilePath} must be true or false.`);
  }
}

function requireList(name) {
  const raw = requireString(name);
  return raw
    .split(/[,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function buildHttpUrl(host, port) {
  return `http://${host}:${port}`;
}

const host = requireString('TEST_HOST');
const routingMode = requireString('TEST_ROUTING_MODE');
const serverPort = requireInteger('TEST_SERVER_PORT', 1);
const clientPort = requireInteger('TEST_CLIENT_PORT', 1);
const loadbotClientPort = requireInteger('TEST_LOADBOT_CLIENT_PORT', 1);
const mapRootDir = path.join(rootDir, 'server', 'data', 'map');

export const testConfig = {
  rootDir,
  testDir,
  envFilePath,
  mapRootDir,
  host,
  regionDir: resolveMapRelativePath('TEST_REGION_DIR', ''),
  server: {
    host,
    port: serverPort,
    listenAddr: `${host}:${serverPort}`,
    url: buildHttpUrl(host, serverPort),
    routingMode,
  },
  client: {
    host,
    port: clientPort,
    url: buildHttpUrl(host, clientPort),
  },
  e2e: {
    timeoutMs: requireInteger('TEST_E2E_TIMEOUT_MS', 1),
    expectTimeoutMs: requireInteger('TEST_E2E_EXPECT_TIMEOUT_MS', 1),
    serverReadyTimeoutMs: requireInteger('TEST_E2E_SERVER_READY_TIMEOUT_MS', 1),
    clientReadyTimeoutMs: requireInteger('TEST_E2E_CLIENT_READY_TIMEOUT_MS', 1),
    actionTimeoutMs: requireInteger('TEST_E2E_ACTION_TIMEOUT_MS', 1),
    candidateAttempts: requireInteger('TEST_E2E_CANDIDATE_ATTEMPTS', 1),
    tripInsetFraction: requireFloat('TEST_E2E_TRIP_INSET_FRACTION', 0, 0.49),
    tripCommuteDegrees: requireFloat('TEST_E2E_TRIP_COMMUTE_DEGREES', 0.001),
    loadbotPath: '/public/index.html?loadbot=1',
  },
  tripGeneration: {
    minDistanceSq: requireFloat('TEST_TRIP_MIN_DISTANCE_SQ', 0),
    maxAttempts: requireInteger('TEST_TRIP_MAX_ATTEMPTS', 1),
  },
  loadbot: {
    clientPort: loadbotClientPort,
    clientURL: `${buildHttpUrl(host, loadbotClientPort)}/public/index.html?loadbot=1`,
    clients: requireInteger('TEST_LOADBOT_CLIENTS', 1),
    headless: requireBoolean('TEST_LOADBOT_HEADLESS'),
    commuteDegrees: requireFloat('TEST_LOADBOT_COMMUTE_DEGREES', 0.01),
    insetFraction: requireFloat('TEST_LOADBOT_INSET_FRACTION', 0, 0.4),
    staggerMs: requireInteger('TEST_LOADBOT_STAGGER_MS', 0),
    statsMs: requireInteger('TEST_LOADBOT_STATS_MS', 1_000),
    tripEndPollMs: requireInteger('TEST_LOADBOT_TRIP_END_POLL_MS', 50),
    viewportWidth: requireInteger('TEST_LOADBOT_VIEWPORT_WIDTH', 1),
    viewportHeight: requireInteger('TEST_LOADBOT_VIEWPORT_HEIGHT', 1),
    blockedAssetPatterns: requireList('TEST_LOADBOT_BLOCKED_ASSETS'),
  },
};
