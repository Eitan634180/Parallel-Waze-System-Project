const junctionState = new Map();

const MAX_IDLE_MS = 30000;

export function reserveJunctionPassage(step, requestedDelayMs) {
    if (!step || requestedDelayMs <= 0) {
        return 0;
    }

    const key = junctionKey(step);
    const now = performance.now();
    pruneIdle(now);

    const state = junctionState.get(key) || { nextAvailableMs: now, lastTouchedMs: now };
    const waitMs = Math.max(0, state.nextAvailableMs - now);
    state.nextAvailableMs = Math.max(now, state.nextAvailableMs) + requestedDelayMs;
    state.lastTouchedMs = now;
    junctionState.set(key, state);
    return waitMs + requestedDelayMs;
}

function junctionKey(step) {
    const [lon, lat] = step.to_node || [];
    return `${lat?.toFixed?.(5) ?? '0'}:${lon?.toFixed?.(5) ?? '0'}`;
}

function pruneIdle(now) {
    for (const [key, state] of junctionState.entries()) {
        if (now - state.lastTouchedMs > MAX_IDLE_MS && state.nextAvailableMs <= now) {
            junctionState.delete(key);
        }
    }
}
