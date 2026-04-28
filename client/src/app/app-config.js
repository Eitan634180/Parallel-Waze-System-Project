import { CLIENT_RUNTIME_CONFIG } from './runtime-config.js';

export const SERVER_URL = CLIENT_RUNTIME_CONFIG.serverUrl;

export const APP_TIMINGS = {
  searchDebounceMs: 400,
  simulationTickMs: 100,
  websocketPingIntervalMs: 1000,
};

export const APP_DEFAULTS = {
  defaultSpeedLimitMps: 13.8,
};
