import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

import { chromium } from 'playwright';

import { runBot } from './bot-runner.mjs';
import { loadConfig } from './config.mjs';
import { startStaticServer } from './static-server.mjs';
import { createTripGenerator, fetchBBox } from './trip-generator.mjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const root = path.resolve(__dirname, '..', '..', '..');
const clientRoot = path.join(root, 'client');

const config = loadConfig(process.argv.slice(2));

let stopping = false;
let activeDrives = 0;
let completedTrips = 0;
let failedTrips = 0;

const bbox = await fetchBBox(config.server);
const nextTrip = createTripGenerator(bbox, config.insetFraction, config.commuteDegrees);
const staticServer = await startStaticServer(clientRoot, config.clientPort);
const browser = await chromium.launch({
  headless: config.headless,
  args: [
    '--disable-background-networking',
    '--disable-component-update',
    '--disable-default-apps',
    '--disable-extensions',
    '--disable-gpu',
    '--disable-sync',
    '--metrics-recording-only',
    '--mute-audio',
    '--no-default-browser-check',
    '--no-first-run',
  ],
});
const clientURL = `http://127.0.0.1:${config.clientPort}/navigation.html?loadbot=1`;

process.on('SIGINT', stop);
process.on('SIGTERM', stop);

const statsTimer = setInterval(printStats, config.statsMs);
const workers = Array.from({ length: config.clients }, (_, index) => launchBot(index + 1));

await Promise.allSettled(workers);
await shutdown();

async function launchBot(id) {
  if (config.staggerMs > 0) {
    await sleep((id - 1) * config.staggerMs);
  }
  if (stopping) {
    return;
  }

  return runBot({
    browser,
    clientURL,
    id,
    nextTrip,
    shouldStop: () => stopping,
    onTripEvent: handleTripEvent,
  });
}

function handleTripEvent(type) {
  switch (type) {
    case 'start':
      activeDrives += 1;
      break;
    case 'complete':
      completedTrips += 1;
      activeDrives = Math.max(0, activeDrives - 1);
      break;
    case 'failure':
      failedTrips += 1;
      activeDrives = Math.max(0, activeDrives - 1);
      break;
    default:
      break;
  }
}

function printStats() {
  console.log(
    `loadbot: active=${activeDrives} completed=${completedTrips} failed=${failedTrips} pages=${config.clients}`,
  );
}

function stop() {
  stopping = true;
}

async function shutdown() {
  clearInterval(statsTimer);
  await browser.close();
  await new Promise((resolve, reject) => staticServer.close((error) => (error ? reject(error) : resolve())));
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
