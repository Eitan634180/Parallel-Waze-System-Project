export function loadConfig(argv) {
  const args = parseArgs(argv);
  return {
    server: args.server || 'http://127.0.0.1:8080',
    clientPort: Number(args['client-port'] || 4173),
    clients: Math.max(1, Number(args.clients || 10)),
    headless: args.headless !== 'false',
    commuteDegrees: Math.max(0.01, Number(args['commute-deg'] || 0.25)),
    insetFraction: clamp(Number(args.inset || 0.1), 0, 0.4),
    staggerMs: Math.max(0, Number(args.stagger || 0)),
    statsMs: Math.max(1000, Number(args.stats || 5000)),
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
