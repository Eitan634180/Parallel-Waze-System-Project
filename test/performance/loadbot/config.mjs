import { testConfig } from '../../config/test-config.mjs';

export function loadConfig(argv) {
  const args = parseArgs(argv);
  return {
    server: args.server || testConfig.server.url,
    clientHost: testConfig.host,
    clientPort: readInteger(args['client-port'], testConfig.loadbot.clientPort, 1),
    clients: readInteger(args.clients, testConfig.loadbot.clients, 1),
    headless: readBoolean(args.headless, testConfig.loadbot.headless),
    commuteDegrees: readFloat(args['commute-deg'], testConfig.loadbot.commuteDegrees, 0.01),
    insetFraction: clamp(readFloat(args.inset, testConfig.loadbot.insetFraction), 0, 0.4),
    staggerMs: readInteger(args.stagger, testConfig.loadbot.staggerMs, 0),
    statsMs: readInteger(args.stats, testConfig.loadbot.statsMs, 1000),
    tripEndPollMs: testConfig.loadbot.tripEndPollMs,
    viewportWidth: testConfig.loadbot.viewportWidth,
    viewportHeight: testConfig.loadbot.viewportHeight,
    blockedAssetPatterns: testConfig.loadbot.blockedAssetPatterns,
    tripGeneration: testConfig.tripGeneration,
  };
}

function parseArgs(argv) {
  const parsed = {};
  for (let i = 0; i < argv.length; i += 1) {
    const current = argv[i];
    if (!current.startsWith('--')) {
      continue;
    }

    const key = current.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith('--')) {
      parsed[key] = 'true';
      continue;
    }

    parsed[key] = next;
    i += 1;
  }
  return parsed;
}

function clamp(value, min, max) {
  if (value < min) return min;
  if (value > max) return max;
  return value;
}

function readInteger(raw, fallback, minimum) {
  const parsed = Number.parseInt(raw ?? '', 10);
  if (Number.isNaN(parsed)) {
    return fallback;
  }
  return Math.max(minimum, parsed);
}

function readFloat(raw, fallback, minimum = null) {
  const parsed = Number.parseFloat(raw ?? '');
  if (Number.isNaN(parsed)) {
    return fallback;
  }
  if (minimum != null && parsed < minimum) {
    return minimum;
  }
  return parsed;
}

function readBoolean(raw, fallback) {
  if (raw == null || raw === '') {
    return fallback;
  }

  switch (String(raw).toLowerCase()) {
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
      return fallback;
  }
}
