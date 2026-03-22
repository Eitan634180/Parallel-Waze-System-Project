import { state, resetDrivingSession } from '../core/state.js';
import { tick, pingRate, DEFAULT_SPEED_LIMIT } from '../core/config.js';
import { mapInstance } from '../ui/map.js';
import { updateDistance, updateETA, updateTrafficStatus, toggleDrivingHUD } from '../ui/ui-hud.js';
import { onArrival, showAlert } from '../ui/ui-alerts.js';
import { renderMainCarDebug } from '../ui/ui-debug.js';
import { createSession, deleteSession } from '../api/api-rest.js';
import { connectToSession, disconnectSession, sendLocationPing } from '../api/api-ws.js';
import { ensureSimulationFeed, getLatestSimulationCars } from './debug-cars.js';
import { processRawRoute, projectPositionOntoRoute } from '../utils/utils.js';
import { advanceSpeedKmh, createDriverProfile, createMotionState, resolveTargetSpeedKmh } from './traffic-model.js';
import { calculateNewPosition, limitMovementByTraffic, calculateBearing } from './driving-physics.js';
import { computeTrafficStatus, getRouteTrafficStatus } from './traffic-evaluator.js';

let drivingWorker = null;
let pingElapsedMs = 0;
let lastTickAt = 0;

function hasEdgeId(edgeId) {
    return edgeId !== null && edgeId !== undefined;
}

function refreshTrafficStatus() {
    const status = computeTrafficStatus();
    updateTrafficStatus(status.level, status.detail);
}

function formatRerouteGainText(oldEtaSec, newEtaSec) {
    if (oldEtaSec == null || newEtaSec == null) {
        return 'Route updated for current traffic conditions.';
    }

    const etaGainSec = Math.max(0, oldEtaSec - newEtaSec);
    if (etaGainSec >= 60) {
        return `New path saves about ${Math.round(etaGainSec / 60)} min.`;
    }
    if (etaGainSec > 0) {
        return `New path saves about ${Math.round(etaGainSec)} sec.`;
    }
    return 'Route updated for current traffic conditions.';
}

function setupDrivingUI() {
    toggleDrivingHUD(true);
    const formatAddress = (addr) => {
        const parts = addr.split(',');
        return parts.length > 2 ? `${parts[0]}, ${parts[1]}` : parts[0];
    };
    document.getElementById('hud-src-name').textContent = formatAddress(state.routing.source.name);
    document.getElementById('hud-dst-name').textContent = formatAddress(state.routing.dest.name);
}

function getCurrentSpeedKmh() {
    if (state.drive.currentRoadIndex >= state.routing.activeLegs.length) {
        return DEFAULT_SPEED_LIMIT * 3.6;
    }
    const currentStep = state.routing.activeLegs[state.drive.currentRoadIndex];
    const recommendedSpeedKmh = hasEdgeId(currentStep.edge_id)
        ? state.sim.recommendedSpeeds.get(currentStep.edge_id)
        : null;
    return recommendedSpeedKmh || currentStep.speed_limit || (DEFAULT_SPEED_LIMIT * 3.6);
}

function handleLocationPing(pos, speedKmh) {
    if (pingElapsedMs >= pingRate) {
        const stepIndex = Math.min(state.drive.currentRoadIndex + 1, state.routing.activeObj.steps.length - 1);
        sendLocationPing(pos[0], pos[1], speedKmh, stepIndex, state.sim.pendingEdgeEvents);
        state.sim.pendingEdgeEvents = [];
        pingElapsedMs %= pingRate;
    }
}

function ensureDrivingWorker() {
    if (drivingWorker) return drivingWorker;

    const workerUrl = new URL('./simulation-worker.js', import.meta.url);
    drivingWorker = new Worker(workerUrl);
    
    drivingWorker.onmessage = (event) => {
        if (event.data?.type !== 'tick') return;

        const now = event.data.now;
        const elapsedMs = lastTickAt === 0 ? tick : Math.max(tick, now - lastTickAt);
        lastTickAt = now;

        try {
            updateSimulation(elapsedMs);
        } catch (err) {
            console.error(err);
            void stopDriving();
        }
    };

    return drivingWorker;
}

export async function startDriving() {
    if (state.routing.activeLegs.length === 0 || state.drive.isActive) return;
    state.sim.recommendedSpeeds.clear();
    mapInstance.drawAlternatives([]);
    let sessionId = null;

    try {
        sessionId = await createSession(state.routing.activeObj.id);
        state.drive.sessionId = sessionId;
        await connectToSession(sessionId, {
            onEtaUpdate: ({ eta_sec }) => {
                updateETA(eta_sec);
            },
            onReroute: (message) => {
                onReroute(message);
            },
            onSpeedUpdate: ({ edge_id, recommended_speed_kmh }) => {
                if (edge_id !== null && edge_id !== undefined) {
                    state.sim.recommendedSpeeds.set(edge_id, recommended_speed_kmh);
                }
                refreshTrafficStatus();
            },
            onDebugUpdate: (debug) => {
                state.debug.serverData = debug;
                renderMainCarDebug(debug);
                refreshTrafficStatus();
            },
            onClose: () => {
                if (!state.drive.isActive || state.drive.sessionId !== sessionId) return;
                showAlert('Navigation disconnected', 'Live server connection was lost.');
                void stopDriving();
            },
        });
    } catch (err) {
        state.drive.sessionId = null;
        if (sessionId) {
            try {
                await deleteSession(sessionId);
            } catch (cleanupErr) {
                console.error('Failed to clean up navigation session after startup error:', cleanupErr);
            }
        }
        console.error('Failed to start navigation session:', err);
        showAlert("Navigation failed", err.message);
        return;
    }

    setupDrivingUI();

    state.drive.currentRoadIndex = 0;
    state.drive.stepProgress = 0;
    state.drive.isActive = true;
    state.drive.distanceLeft = state.routing.activeObj.distance;
    state.drive.totalDistanceDrivenM = 0;
    state.drive.startTimeMs = Date.now();
    state.sim.pendingEdgeEvents = [];
    state.sim.currentEdgeTimeMs = 0;
    state.sim.driverProfile = createDriverProfile();
    state.sim.motionState = createMotionState();
    state.debug.serverData = null;
    state.sim.cars = getLatestSimulationCars();
    renderMainCarDebug(null);
    pingElapsedMs = 0;
    lastTickAt = 0;

    void ensureSimulationFeed().catch((err) => {
        console.error('Optional simulation feed unavailable:', err);
    });

    const firstCoord = state.routing.activeLegs[0].from_node;
    state.drive.carPos = [firstCoord[1], firstCoord[0]];
    mapInstance.initCarMarker(state.drive.carPos);

    ensureDrivingWorker().postMessage({ type: 'start', tickMs: tick });
}

function updateSimulation(elapsedMs) {
    if (state.drive.currentRoadIndex >= state.routing.activeLegs.length) {
        const actualTimeSec = state.drive.startTimeMs 
            ? (Date.now() - state.drive.startTimeMs) / 1000 
            : state.routing.activeObj.dynamicETA;

        const totalDistanceM = state.drive.totalDistanceDrivenM || state.routing.activeObj.distance;
        void stopDriving();
        onArrival(totalDistanceM, actualTimeSec);
        return;
    }

    const speedKmhValue = getCurrentSpeedKmh();
    pingElapsedMs += elapsedMs;
    state.sim.currentEdgeTimeMs += elapsedMs;

    const activeEdgeId = state.routing.activeLegs[state.drive.currentRoadIndex]?.edge_id;
    const targetSpeedKmh = resolveTargetSpeedKmh(
        state.routing.activeLegs[state.drive.currentRoadIndex],
        hasEdgeId(activeEdgeId) ? state.sim.recommendedSpeeds.get(activeEdgeId) : null,
        state.sim.driverProfile,
    );
    state.sim.motionState.speedKmh = advanceSpeedKmh(state.sim.motionState.speedKmh || speedKmhValue, targetSpeedKmh, elapsedMs, state.sim.driverProfile);
    state.sim.cars = getLatestSimulationCars();
    const unclampedSpeedKmh = state.sim.motionState.speedKmh;
    const unclampedMetersThisTick = (unclampedSpeedKmh / 3.6) * (elapsedMs / 1000);
    const metersThisTick = limitMovementByTraffic(unclampedMetersThisTick, unclampedSpeedKmh);
    if (unclampedMetersThisTick > 0 && metersThisTick < unclampedMetersThisTick) {
        state.sim.motionState.speedKmh = (metersThisTick / (elapsedMs / 1000)) * 3.6;
    }
    const speedKmh = Math.round(state.sim.motionState.speedKmh);

    state.drive.distanceLeft = Math.max(0, state.drive.distanceLeft - metersThisTick);
    state.drive.totalDistanceDrivenM += metersThisTick;

    state.drive.carPos = calculateNewPosition(metersThisTick, elapsedMs);
    const bearing = calculateBearing(state.drive.carPos);

    mapInstance.updateCarPositionAndRotation([state.drive.carPos[0], state.drive.carPos[1]], bearing);

    handleLocationPing(state.drive.carPos, speedKmh);

    updateDistance(state.drive.distanceLeft, 'hud-distance-left');
    document.getElementById('hud-speed').textContent = `${speedKmh} km / h`;
}

export async function stopDriving() {
    state.drive.isActive = false;
    if (drivingWorker) {
        drivingWorker.postMessage({ type: 'stop' });
    }
    pingElapsedMs = 0;
    lastTickAt = 0;

    toggleDrivingHUD(false);
    mapInstance.removeCarMarker();

    if (state.routing.activeObj) {
        document.getElementById('route-panel').classList.remove('hidden');
    }
    document.getElementById('search-panel').classList.remove('hidden');

    disconnectSession();
    const sessionId = state.drive.sessionId;
    resetDrivingSession();
    renderMainCarDebug(null);
    if (state.routing.activeObj) {
        const traffic = getRouteTrafficStatus(state.routing.activeObj);
        updateTrafficStatus(traffic.level, traffic.detail);
    } else {
        updateTrafficStatus('normal');
    }

    if (sessionId) {
        try {
            await deleteSession(sessionId);
        } catch (err) {
            console.error('Failed to delete navigation session:', err);
        }
    }
}

export function onReroute(message) {

    state.drive.offRouteOffset = [0, 0];
    state.drive.isDrifting = false;

    const routeObj = processRawRoute(message.route);
    const oldRouteObj = state.routing.activeObj;

    const projected = state.drive.carPos
        ? projectPositionOntoRoute(routeObj, state.drive.carPos[0], state.drive.carPos[1])
        : null;
    const reason = message.reroute_reason || 'traffic';
    const oldEtaSec = message.old_eta_sec;
    const newEtaSec = message.new_eta_sec;

    if (state.debug.serverData) {
        state.debug.serverData.last_reroute_reason = reason;
        state.debug.serverData.last_reroute_at_unix_ms = Date.now();
        renderMainCarDebug(state.debug.serverData);
    }

    if (reason === 'off_route') {
        const dist = state.debug.serverData?.off_route_distance_m;
        const distText = typeof dist === 'number' ? `${dist.toFixed(1)} m away from the expected path.` : 'Vehicle left the expected path.';
        showAlert('Off-route reroute', distText);
    } else {
        showAlert('Traffic reroute', formatRerouteGainText(oldEtaSec, newEtaSec));
    }
    state.sim.recommendedSpeeds.clear();
    state.sim.currentEdgeTimeMs = 0;

    state.routing.activeObj = routeObj;
    state.routing.activeLegs = routeObj.legs; 
    updateETA(message.new_eta_sec ?? routeObj.dynamicETA);

    if (state.drive.isActive) {
        state.sim.pendingEdgeEvents = [];
        if (projected) {
            state.drive.currentRoadIndex = projected.roadIndex;
            state.drive.stepProgress = projected.stepProgress;
            state.drive.distanceLeft = projected.distanceLeft;
            state.drive.carPos = [projected.lat, projected.lng];
        } else {
            state.drive.currentRoadIndex = 0;
            state.drive.stepProgress = 0;
            state.drive.distanceLeft = routeObj.distance;
        }
        if (!state.sim.driverProfile) {
            state.sim.driverProfile = createDriverProfile();
        }
        if (!state.sim.motionState) {
            state.sim.motionState = createMotionState();
        }
        updateDistance(state.drive.distanceLeft, 'hud-distance-left');
    } else {
        updateDistance(routeObj.distance, 'hud-distance-left');
    }

    if (oldRouteObj) {
        mapInstance.drawAlternatives([oldRouteObj]);
    }
    
    mapInstance.drawRoute(routeObj.pathCoords, state.routing.source, state.routing.dest);
    mapInstance.setRouteInspectorRoute(routeObj);
    if (state.drive.isActive && state.drive.carPos) {
        const bearing = calculateBearing(state.drive.carPos);
        mapInstance.updateCarPositionAndRotation([state.drive.carPos[0], state.drive.carPos[1]], bearing);
        const stepIndex = Math.min(state.drive.currentRoadIndex + 1, state.routing.activeObj.steps.length - 1);
        sendLocationPing(state.drive.carPos[0], state.drive.carPos[1], Math.round(getCurrentSpeedKmh()), stepIndex, []);
    }
    refreshTrafficStatus();
}
