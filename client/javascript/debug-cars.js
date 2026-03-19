import { clearSimulationCars, connectToSimulation, disconnectSimulation, spawnRandomSimulationCars, spawnSimulationCars } from './api.js';
import { mapInstance } from './map.js';

const listeners = new Set();
let createdCount = 0;
let activeCount = 0;
let completedCount = 0;
let simulationReadyPromise = null;
let latestCars = [];

function notify() {
    listeners.forEach((listener) => listener({
        active: activeCount,
        created: createdCount,
        completed: completedCount,
    }));
}

async function ensureSimulationFeed() {
    if (simulationReadyPromise) {
        return simulationReadyPromise;
    }

    simulationReadyPromise = connectToSimulation({
        onSnapshot: (cars) => {
            latestCars = cars || [];
            const nextActive = cars.length;
            if (nextActive < activeCount) {
                completedCount += (activeCount - nextActive);
            }
            activeCount = nextActive;
            mapInstance.syncDebugCars(cars);
            notify();
        },
    }).catch((err) => {
        simulationReadyPromise = null;
        throw err;
    });

    return simulationReadyPromise;
}

export async function spawnDebugCars(routeObj, count, routePool = [], minStepIndex = 0) {
    if (!routeObj || !routeObj.id || !routeObj.legs.length) {
        throw new Error('Select a route before adding cars.');
    }

    await ensureSimulationFeed();
    const routeIds = (routePool || [])
        .map((route) => route?.id)
        .filter(Boolean);
    if (!routeIds.length) {
        routeIds.push(routeObj.id);
    }

    const result = await spawnSimulationCars(routeIds, count, minStepIndex);
    createdCount += result.created || 0;
    activeCount = result.active || activeCount;
    notify();
}

export async function spawnRandomDebugCars(count) {
    await ensureSimulationFeed();
    const result = await spawnRandomSimulationCars(count);
    createdCount += result.created || 0;
    activeCount = result.active || activeCount;
    notify();
}

export async function clearDebugCars() {
    await clearSimulationCars();
    activeCount = 0;
    mapInstance.clearDebugCars();
    notify();
}

export function subscribeDebugCars(listener) {
    listeners.add(listener);
    listener({
        active: activeCount,
        created: createdCount,
        completed: completedCount,
    });
    return () => listeners.delete(listener);
}

export async function ensureSimulationCarsFeed() {
    await ensureSimulationFeed();
}

export function getLatestSimulationCars() {
    return latestCars.slice();
}

export function disconnectDebugCars() {
    disconnectSimulation();
    simulationReadyPromise = null;
    latestCars = [];
    activeCount = 0;
    mapInstance.clearDebugCars();
    notify();
}
