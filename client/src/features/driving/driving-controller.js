import { state, resetDrivingSession } from '../../app/app-state.js';
import { DEFAULT_SPEED_LIMIT } from '../../app/app-config.js';
import { mapInstance } from '../../ui/map/map-manager.js';
import { updateDistance, updateETA, updateTrafficStatus, toggleDrivingHUD } from '../../ui/panels/hud-panel.js';
import { onArrival, showAlert } from '../../ui/panels/alerts-panel.js';
import { renderMainCarDebug } from '../../ui/panels/debug-panel.js';
import { ensureSimulationFeed, getLatestSimulationCars } from '../debug/debug-cars-service.js';
import { advanceSpeedKmh, createDriverProfile, createMotionState, resolveTargetSpeedKmh } from './traffic-model.js';
import { calculateBearing, calculateNewPosition, limitMovementByTraffic } from './driving-physics.js';
import { computeTrafficStatus, getRouteTrafficStatus } from './traffic-evaluator.js';
import { handleDrivingReroute } from './driving-reroute.js';
import { closeDrivingSession, openDrivingSession } from './driving-session.js';
import { accumulatePingElapsed, flushLocationPing, resetDrivingRuntime, startDrivingRuntime, stopDrivingRuntime } from './driving-runtime.js';

function hasEdgeId(edgeId) {
    return edgeId !== null && edgeId !== undefined;
}

function refreshTrafficStatus() {
    const status = computeTrafficStatus();
    updateTrafficStatus(status.level, status.detail);
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

export async function startDriving() {
    if (state.routing.activeLegs.length === 0 || state.drive.isActive) return;
    state.sim.recommendedSpeeds.clear();
    mapInstance.drawAlternatives([]);

    try {
        state.drive.sessionId = await openDrivingSession(state.routing.activeObj.id, {
            onEtaUpdate: ({ eta_sec }) => {
                updateETA(eta_sec);
            },
            onReroute: (message) => {
                handleDrivingReroute(message, {
                    getBearing: calculateBearing,
                    getCurrentSpeedKmh,
                    refreshTrafficStatus,
                });
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
                if (!state.drive.isActive) return;
                showAlert('Navigation disconnected', 'Live server connection was lost.');
                void stopDriving();
            },
        });
    } catch (err) {
        state.drive.sessionId = null;
        console.error('Failed to start navigation session:', err);
        showAlert('Navigation failed', err.message);
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
    resetDrivingRuntime();

    void ensureSimulationFeed().catch((err) => {
        console.error('Optional simulation feed unavailable:', err);
    });

    const firstCoord = state.routing.activeLegs[0].from_node;
    state.drive.carPos = [firstCoord[1], firstCoord[0]];
    mapInstance.initCarMarker(state.drive.carPos);

    startDrivingRuntime(
        (elapsedMs) => updateSimulation(elapsedMs),
        (error) => {
            console.error(error);
            void stopDriving();
        },
    );
}

function updateSimulation(elapsedMs) {
    if (!state.drive.isActive || !state.routing.activeObj || !state.sim.motionState) {
        return;
    }

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
    accumulatePingElapsed(elapsedMs);
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

    state.sim.pendingEdgeEvents = flushLocationPing(
        state.routing.activeObj,
        state.drive.currentRoadIndex,
        state.drive.carPos,
        speedKmh,
        state.sim.pendingEdgeEvents,
    );

    updateDistance(state.drive.distanceLeft, 'hud-distance-left');
    document.getElementById('hud-speed').textContent = `${speedKmh} km / h`;
}

export async function stopDriving() {
    state.drive.isActive = false;
    stopDrivingRuntime();

    toggleDrivingHUD(false);
    mapInstance.removeCarMarker();

    if (state.routing.activeObj) {
        document.getElementById('route-panel').classList.remove('hidden');
    }
    document.getElementById('search-panel').classList.remove('hidden');

    const sessionId = state.drive.sessionId;
    resetDrivingSession();
    renderMainCarDebug(null);
    if (state.routing.activeObj) {
        const traffic = getRouteTrafficStatus(state.routing.activeObj);
        updateTrafficStatus(traffic.level, traffic.detail);
    } else {
        updateTrafficStatus('normal');
    }

    await closeDrivingSession(sessionId);
}
