import { APP_DEFAULTS } from '../../app/app-config.js';
import { DEGREES_PER_RADIAN, KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND } from '../../utils/math.js';
import { SECONDS_PER_MILLISECOND } from '../../utils/time.js';
import {
    DRIVER_PROFILE_RANGES,
    DRIVING_SPEED,
    INTERSECTION_DELAYS,
} from './config.js';

const MIN_COS_THETA = -1;
const MAX_COS_THETA = 1;

const DRIVER_PROFILE_SEEDS = {
    accelerationMps2: 29.1,
    brakingMps2: 43.7,
    junctionBias: 61.9,
    paceBias: 17.3,
};

export function createDriverProfile(seed = Math.random()) {
    const paceBias = seededRange(seed * DRIVER_PROFILE_SEEDS.paceBias, ...DRIVER_PROFILE_RANGES.paceBias);
    const accelMs2 = seededRange(seed * DRIVER_PROFILE_SEEDS.accelerationMps2, ...DRIVER_PROFILE_RANGES.accelerationMps2);
    const brakeMs2 = seededRange(seed * DRIVER_PROFILE_SEEDS.brakingMps2, ...DRIVER_PROFILE_RANGES.brakingMps2);
    const junctionBias = seededRange(seed * DRIVER_PROFILE_SEEDS.junctionBias, ...DRIVER_PROFILE_RANGES.junctionBias);

    return {
        paceBias,
        accelMs2,
        brakeMs2,
        junctionBias,
    };
}

export function createMotionState() {
    return {
        speedKmh: APP_DEFAULTS.defaultSpeedLimitMps * KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND,
    };
}

export function resolveTargetSpeedKmh(step, recommendedSpeedKmh, profile) {
    const baseSpeed = recommendedSpeedKmh || step?.speed_limit || (APP_DEFAULTS.defaultSpeedLimitMps * KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND);
    const target = baseSpeed * (profile?.paceBias || 1);
    return clamp(
        target,
        DRIVING_SPEED.minTargetSpeedKmh,
        Math.max(DRIVING_SPEED.minTargetSpeedCapKmh, baseSpeed * DRIVING_SPEED.targetSpeedBufferRatio),
    );
}

export function advanceSpeedKmh(currentKmh, targetKmh, elapsedMs, profile) {
    const currentMs = currentKmh / KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND;
    const targetMs = targetKmh / KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND;
    const delta = targetMs - currentMs;
    const accel = delta >= 0
        ? (profile?.accelMs2 || DRIVING_SPEED.defaultAccelerationMps2)
        : (profile?.brakeMs2 || DRIVING_SPEED.defaultBrakingMps2);
    const maxDelta = accel * (elapsedMs * SECONDS_PER_MILLISECOND);
    const nextMs = currentMs + clamp(delta, -maxDelta, maxDelta);
    return Math.max(0, nextMs * KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND);
}

export function computeIntersectionDelayMs(route, roadIndex, profile) {
    if (!route?.length || roadIndex < 0 || roadIndex >= route.length - 1) {
        return 0;
    }

    const current = route[roadIndex];
    const next = route[roadIndex + 1];
    const { angle } = getTurnInfo(current, next);
    const nextSpeed = next?.speed_limit || current?.speed_limit || (APP_DEFAULTS.defaultSpeedLimitMps * KILOMETERS_PER_HOUR_TO_METERS_PER_SECOND);

    let delayMs = 0;
    if (angle > INTERSECTION_DELAYS.sharpTurnAngleDeg) {
        delayMs = INTERSECTION_DELAYS.sharpTurnDelayMs;
    } else if (angle > INTERSECTION_DELAYS.turnAngleDeg) {
        delayMs = INTERSECTION_DELAYS.turnDelayMs;
    } else if (nextSpeed <= INTERSECTION_DELAYS.lowSpeedRoadThresholdKmh) {
        delayMs = INTERSECTION_DELAYS.lowSpeedRoadDelayMs;
    } else if (current?.base_length < INTERSECTION_DELAYS.shortSegmentLengthM || next?.base_length < INTERSECTION_DELAYS.shortSegmentLengthM) {
        delayMs = INTERSECTION_DELAYS.shortSegmentDelayMs;
    }

    if (delayMs === 0) {
        return 0;
    }

    return Math.round(Math.max(INTERSECTION_DELAYS.minDelayMs, delayMs * (profile?.junctionBias || 1)));
}

export function getTurnInfo(current, next) {
    if (!current || !next) {
        return { angle: 0, direction: 0 };
    }

    const ax = current.to_node[0] - current.from_node[0];
    const ay = current.to_node[1] - current.from_node[1];
    const bx = next.to_node[0] - next.from_node[0];
    const by = next.to_node[1] - next.from_node[1];

    const magA = Math.hypot(ax, ay);
    const magB = Math.hypot(bx, by);
    if (magA === 0 || magB === 0) {
        return { angle: 0, direction: 0 };
    }

    const dot = ((ax * bx) + (ay * by)) / (magA * magB);
    const cosTheta = clamp(dot, MIN_COS_THETA, MAX_COS_THETA);
    const angle = Math.acos(cosTheta) * DEGREES_PER_RADIAN;

    const cross = ax * by - ay * bx;
    const direction = cross > 0 ? 1 : -1;

    return { angle, direction };
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
