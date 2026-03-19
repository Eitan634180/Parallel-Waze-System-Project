import { DEFAULT_SPEED_LIMIT } from './config.js';

const MIN_DELAY_MS = 250;

export function createDriverProfile(seed = Math.random()) {
    const paceBias = seededRange(seed * 17.3, 0.85, 1.15);
    const accelMs2 = seededRange(seed * 29.1, 1.2, 2.8);
    const brakeMs2 = seededRange(seed * 43.7, 1.8, 3.6);
    const junctionBias = seededRange(seed * 61.9, 0.85, 1.3);

    return {
        paceBias,
        accelMs2,
        brakeMs2,
        junctionBias,
    };
}

export function createMotionState() {
    return {
        speedKmh: DEFAULT_SPEED_LIMIT * 3.6,
        waitingMs: 0,
    };
}

export function resolveTargetSpeedKmh(step, recommendedSpeedKmh, profile) {
    const baseSpeed = recommendedSpeedKmh || step?.speed_limit || (DEFAULT_SPEED_LIMIT * 3.6);
    const target = baseSpeed * (profile?.paceBias || 1);
    return clamp(target, 0.5, Math.max(8, baseSpeed * 1.05));
}

export function advanceSpeedKmh(currentKmh, targetKmh, elapsedMs, profile) {
    const currentMs = currentKmh / 3.6;
    const targetMs = targetKmh / 3.6;
    const delta = targetMs - currentMs;
    const accel = delta >= 0 ? (profile?.accelMs2 || 2.0) : (profile?.brakeMs2 || 3.0);
    const maxDelta = accel * (elapsedMs / 1000);
    const nextMs = currentMs + clamp(delta, -maxDelta, maxDelta);
    return Math.max(0, nextMs * 3.6);
}

export function computeIntersectionDelayMs(route, roadIndex, profile) {
    if (!route?.length || roadIndex < 0 || roadIndex >= route.length - 1) {
        return 0;
    }

    const current = route[roadIndex];
    const next = route[roadIndex + 1];
    const angle = turnAngleDeg(current, next);
    const nextSpeed = next?.speed_limit || current?.speed_limit || (DEFAULT_SPEED_LIMIT * 3.6);

    let delayMs = 0;
    if (angle > 120) {
        delayMs = 2200;
    } else if (angle > 65) {
        delayMs = 1200;
    } else if (nextSpeed <= 35) {
        delayMs = 550;
    } else if (current?.base_length < 35 || next?.base_length < 35) {
        delayMs = 300;
    }

    if (delayMs === 0) {
        return 0;
    }

    return Math.round(Math.max(MIN_DELAY_MS, delayMs * (profile?.junctionBias || 1)));
}

function turnAngleDeg(current, next) {
    if (!current || !next) {
        return 0;
    }

    const ax = current.to_node[0] - current.from_node[0];
    const ay = current.to_node[1] - current.from_node[1];
    const bx = next.to_node[0] - next.from_node[0];
    const by = next.to_node[1] - next.from_node[1];
    const magA = Math.hypot(ax, ay);
    const magB = Math.hypot(bx, by);
    if (magA === 0 || magB === 0) {
        return 0;
    }

    const dot = ((ax * bx) + (ay * by)) / (magA * magB);
    const cosTheta = clamp(dot, -1, 1);
    return Math.acos(cosTheta) * 180 / Math.PI;
}

function seededRange(seed, min, max) {
    const normalized = Math.abs(Math.sin(seed)) % 1;
    return min + (max - min) * normalized;
}

function clamp(value, min, max) {
    if (value < min) return min;
    if (value > max) return max;
    return value;
}
