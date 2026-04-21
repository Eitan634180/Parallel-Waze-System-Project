import {
    spawnSimulationCars,
    spawnRandomSimulationCars,
    clearSimulationCars,
} from '../../services/rest/navigation-api.js';
import { connectToSimulation, disconnectSimulation } from '../../services/ws/socket-client.js';
import { mapInstance } from '../../ui/map/map-manager.js';
import { METERS_PER_DEGREE_LATITUDE, RADIANS_PER_DEGREE } from '../../utils/math.js';
import { DEBUG_CARS } from './config.js';

let activeCount = 0;
let simulationReadyPromise = null;
let cars = [];
let onUpdateCallback = null;
let pendingSnapshotFrame = 0;
let trafficBuckets = new Map();
let lonMetersPerDegree = METERS_PER_DEGREE_LATITUDE;

function flushSimulationSnapshot() {
    pendingSnapshotFrame = 0;
    if (mapInstance.debugCarsVisible) {
        mapInstance.syncDebugCars(cars);
    }
}

function scheduleSimulationSnapshotFlush() {
    if (pendingSnapshotFrame) {
        return;
    }
    pendingSnapshotFrame = requestAnimationFrame(flushSimulationSnapshot);
}

function cancelPendingSnapshotFlush() {
    if (!pendingSnapshotFrame) {
        return;
    }
    cancelAnimationFrame(pendingSnapshotFrame);
    pendingSnapshotFrame = 0;
}

function clearSimulationSnapshots() {
    cancelPendingSnapshotFlush();
    cars = [];
    trafficBuckets = new Map();
    activeCount = 0;
    mapInstance.clearDebugCars();
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}

function rebuildTrafficBuckets(snapshotCars) {
    trafficBuckets = new Map();
    if (!snapshotCars.length) {
        lonMetersPerDegree = METERS_PER_DEGREE_LATITUDE;
        return;
    }

    const averageLat = snapshotCars.reduce((sum, car) => sum + car.lat, 0) / snapshotCars.length;
    lonMetersPerDegree = Math.max(1, METERS_PER_DEGREE_LATITUDE * Math.cos(averageLat * RADIANS_PER_DEGREE));

    for (const car of snapshotCars) {
        const key = bucketKey(car.lat, car.lon);
        const bucket = trafficBuckets.get(key);
        if (bucket) {
            bucket.push(car);
        } else {
            trafficBuckets.set(key, [car]);
        }
    }
}

function bucketKey(lat, lon) {
    const latBucket = Math.floor((lat * METERS_PER_DEGREE_LATITUDE) / DEBUG_CARS.trafficBucketSizeM);
    const lonBucket = Math.floor((lon * lonMetersPerDegree) / DEBUG_CARS.trafficBucketSizeM);
    return `${latBucket}:${lonBucket}`;
}

function resetSimulationFeedState() {
    simulationReadyPromise = null;
    clearSimulationSnapshots();
}

export function setDebugCarCallback(callback) {
    onUpdateCallback = callback;
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}

export async function ensureSimulationFeed() {
    if (simulationReadyPromise) return simulationReadyPromise;

    simulationReadyPromise = connectToSimulation({
        onSnapshot: (snapshotCars) => {
            cars = snapshotCars || [];
            rebuildTrafficBuckets(cars);
            activeCount = cars.length;
            if (onUpdateCallback) onUpdateCallback({ active: activeCount });
            scheduleSimulationSnapshotFlush();
        },
        onClose: () => {
            resetSimulationFeedState();
        },
    }).catch((err) => {
        resetSimulationFeedState();
        throw err;
    });

    return simulationReadyPromise;
}

export async function spawnDebugCars(routeObj, count, minStepIndex = 0) {
    if (!routeObj || !routeObj.id || !routeObj.legs.length) throw new Error('Select a route before adding cars.');

    await ensureSimulationFeed();
    const result = await spawnSimulationCars([routeObj.id], count, minStepIndex);
    activeCount = result.active || activeCount;
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}

export async function spawnRandomDebugCars(count) {
    await ensureSimulationFeed();
    const result = await spawnRandomSimulationCars(count);
    activeCount = result.active || activeCount;
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}

export async function clearDebugCars() {
    await clearSimulationCars();
    clearSimulationSnapshots();
}

export function getNearbySimulationCars(lat, lon) {
    if (!Number.isFinite(lat) || !Number.isFinite(lon) || trafficBuckets.size === 0) {
        return cars;
    }

    const latBucket = Math.floor((lat * METERS_PER_DEGREE_LATITUDE) / DEBUG_CARS.trafficBucketSizeM);
    const lonBucket = Math.floor((lon * lonMetersPerDegree) / DEBUG_CARS.trafficBucketSizeM);
    const nearbyCars = [];

    for (let latOffset = -DEBUG_CARS.nearbyBucketRadius; latOffset <= DEBUG_CARS.nearbyBucketRadius; latOffset++) {
        for (let lonOffset = -DEBUG_CARS.nearbyBucketRadius; lonOffset <= DEBUG_CARS.nearbyBucketRadius; lonOffset++) {
            const bucket = trafficBuckets.get(`${latBucket + latOffset}:${lonBucket + lonOffset}`);
            if (bucket) {
                nearbyCars.push(...bucket);
            }
        }
    }

    return nearbyCars;
}

export function syncStoredDebugCars() {
    cancelPendingSnapshotFlush();
    mapInstance.syncDebugCars(cars);
}
