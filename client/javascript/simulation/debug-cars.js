import { spawnSimulationCars, spawnRandomSimulationCars, clearSimulationCars } from '../api/api-rest.js';
import { connectToSimulation, disconnectSimulation } from '../api/api-ws.js';
import { mapInstance } from '../ui/map.js';

let activeCount = 0;
let simulationReadyPromise = null;
let latestCars = [];
let onUpdateCallback = null;

export function setDebugCarCallback(callback) {
    onUpdateCallback = callback;
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}

export async function ensureSimulationFeed() {
    if (simulationReadyPromise) return simulationReadyPromise;

    simulationReadyPromise = connectToSimulation({
        onSnapshot: (cars) => {
            latestCars = cars || [];
            activeCount = latestCars.length;
            mapInstance.syncDebugCars(cars);
            if (onUpdateCallback) onUpdateCallback({ active: activeCount });
        },
    }).catch((err) => {
        simulationReadyPromise = null;
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
    activeCount = 0;
    mapInstance.clearDebugCars();
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
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
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
}