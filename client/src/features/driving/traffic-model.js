import { DEFAULT_SPEED_LIMIT } from '../../app/app-config.js';

const MIN_DELAY_MS = 250;
const KMH_PER_MPS = 3.6;
const SECONDS_PER_MILLISECOND = 1 / 1000;
const MIN_TARGET_SPEED_KMH = 0.5;
const MIN_TARGET_SPEED_CAP_KMH = 8;
const TARGET_SPEED_BUFFER_RATIO = 1.05;
const DEFAULT_ACCELERATION_MPS2 = 2.0;
const DEFAULT_BRAKING_MPS2 = 3.0;
const SHARP_TURN_ANGLE_DEG = 120;
const TURN_ANGLE_DEG = 65;
const LOW_SPEED_ROAD_THRESHOLD_KMH = 35;
const SHORT_SEGMENT_LENGTH_M = 35;
const SHARP_TURN_DELAY_MS = 2200;
const TURN_DELAY_MS = 1200;
const LOW_SPEED_ROAD_DELAY_MS = 550;
const SHORT_SEGMENT_DELAY_MS = 300;
const MIN_COS_THETA = -1;
const MAX_COS_THETA = 1;
const DEGREES_PER_RADIAN = 180 / Math.PI;

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
        speedKmh: DEFAULT_SPEED_LIMIT * KMH_PER_MPS,
    };
}

export function resolveTargetSpeedKmh(step, recommendedSpeedKmh, profile) {
    const baseSpeed = recommendedSpeedKmh || step?.speed_limit || (DEFAULT_SPEED_LIMIT * KMH_PER_MPS);
    const target = baseSpeed * (profile?.paceBias || 1);
    return clamp(target, MIN_TARGET_SPEED_KMH, Math.max(MIN_TARGET_SPEED_CAP_KMH, baseSpeed * TARGET_SPEED_BUFFER_RATIO));
}

export function advanceSpeedKmh(currentKmh, targetKmh, elapsedMs, profile) {
    const currentMs = currentKmh / KMH_PER_MPS;
    const targetMs = targetKmh / KMH_PER_MPS;
    const delta = targetMs - currentMs;
    const accel = delta >= 0 ? (profile?.accelMs2 || DEFAULT_ACCELERATION_MPS2) : (profile?.brakeMs2 || DEFAULT_BRAKING_MPS2);
    const maxDelta = accel * (elapsedMs * SECONDS_PER_MILLISECOND);
    const nextMs = currentMs + clamp(delta, -maxDelta, maxDelta);
    return Math.max(0, nextMs * KMH_PER_MPS);
}

export function computeIntersectionDelayMs(route, roadIndex, profile) {
    if (!route?.length || roadIndex < 0 || roadIndex >= route.length - 1) {
        return 0;
    }

    const current = route[roadIndex];
    const next = route[roadIndex + 1];
    const angle = turnAngleDeg(current, next);
    const nextSpeed = next?.speed_limit || current?.speed_limit || (DEFAULT_SPEED_LIMIT * KMH_PER_MPS);

    let delayMs = 0;
    if (angle > SHARP_TURN_ANGLE_DEG) {
        delayMs = SHARP_TURN_DELAY_MS;
    } else if (angle > TURN_ANGLE_DEG) {
        delayMs = TURN_DELAY_MS;
    } else if (nextSpeed <= LOW_SPEED_ROAD_THRESHOLD_KMH) {
        delayMs = LOW_SPEED_ROAD_DELAY_MS;
    } else if (current?.base_length < SHORT_SEGMENT_LENGTH_M || next?.base_length < SHORT_SEGMENT_LENGTH_M) {
        delayMs = SHORT_SEGMENT_DELAY_MS;
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
    const cosTheta = clamp(dot, MIN_COS_THETA, MAX_COS_THETA);
    return Math.acos(cosTheta) * DEGREES_PER_RADIAN;
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
