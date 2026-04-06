import { tick, pingRate } from '../../app/app-config.js';
import { sendLocationPing } from '../../services/ws/socket-client.js';

let drivingWorker = null;
let pingElapsedMs = 0;
let lastTickAt = 0;

const NEXT_STEP_OFFSET = 1;
const WORKER_MESSAGE_TYPES = {
    start: 'start',
    stop: 'stop',
    tick: 'tick',
};

export function startDrivingRuntime(onTick, onError) {
    const worker = ensureDrivingWorker(onTick, onError);
    worker.postMessage({ type: WORKER_MESSAGE_TYPES.start, tickMs: tick });
}

export function stopDrivingRuntime() {
    if (drivingWorker) {
        drivingWorker.postMessage({ type: WORKER_MESSAGE_TYPES.stop });
    }
    resetDrivingRuntime();
}

export function resetDrivingRuntime() {
    pingElapsedMs = 0;
    lastTickAt = 0;
}

export function trackElapsedMs(now) {
    const elapsedMs = lastTickAt === 0 ? tick : Math.max(tick, now - lastTickAt);
    lastTickAt = now;
    return elapsedMs;
}

export function flushLocationPing(route, currentRoadIndex, carPos, speedKmh, edgeEvents) {
    if (!route?.steps?.length || pingElapsedMs < pingRate) {
        return edgeEvents;
    }

    const stepIndex = Math.min(currentRoadIndex + NEXT_STEP_OFFSET, route.steps.length - NEXT_STEP_OFFSET);
    sendLocationPing(carPos[0], carPos[1], speedKmh, stepIndex, edgeEvents);
    pingElapsedMs %= pingRate;
    return [];
}

export function accumulatePingElapsed(elapsedMs) {
    pingElapsedMs += elapsedMs;
}

function ensureDrivingWorker(onTick, onError) {
    if (drivingWorker) {
        return drivingWorker;
    }

    const workerUrl = new URL('./simulation-ticker.worker.js', import.meta.url);
    drivingWorker = new Worker(workerUrl);
    drivingWorker.onmessage = (event) => {
        if (event.data?.type !== WORKER_MESSAGE_TYPES.tick) {
            return;
        }

        const elapsedMs = trackElapsedMs(event.data.now);
        try {
            onTick(elapsedMs);
        } catch (error) {
            onError(error);
        }
    };

    return drivingWorker;
}
