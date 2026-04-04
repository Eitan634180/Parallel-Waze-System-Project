import {
    spawnSimulationCars,
    spawnRandomSimulationCars,
    clearSimulationCars,
} from '../../services/rest/navigation-api.js';
import { connectToSimulation, disconnectSimulation } from '../../services/ws/socket-client.js';
import { mapInstance } from '../../ui/map/map-manager.js';

let activeCount = 0;
let simulationReadyPromise = null;
let cars = [];
let onUpdateCallback = null;

function clearSimulationSnapshots() {
    cars = [];
    activeCount = 0;
    mapInstance.clearDebugCars();
    if (onUpdateCallback) onUpdateCallback({ active: activeCount });
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
            activeCount = cars.length;
            mapInstance.syncDebugCars(snapshotCars);
            if (onUpdateCallback) onUpdateCallback({ active: activeCount });
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

export function getLatestSimulationCars() {
    return cars.slice();
}

export function disconnectDebugCars() {
    disconnectSimulation();
    resetSimulationFeedState();
}
