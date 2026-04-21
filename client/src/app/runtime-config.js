function readBrowserRuntimeConfig() {
  if (typeof window !== 'undefined' && window.__NAV_RUNTIME_CONFIG__) {
    return window.__NAV_RUNTIME_CONFIG__;
  }
  return null;
}

function readNodeRuntimeConfig() {
  if (typeof process !== 'undefined' && process?.env) {
    return {
      serverUrl: process.env.NAV_CLIENT_SERVER_URL,
    };
  }
  return null;
}

function readRuntimeConfig() {
  return readBrowserRuntimeConfig() || readNodeRuntimeConfig() || {};
}

function requireRuntimeValue(name) {
  const value = readRuntimeConfig()[name];
  if (!value) {
    throw new Error(`Client runtime config value "${name}" is required.`);
  }
  return value;
}

export const CLIENT_RUNTIME_CONFIG = Object.freeze({
  serverUrl: requireRuntimeValue('serverUrl'),
});
