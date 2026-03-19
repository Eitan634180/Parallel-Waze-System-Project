import { state } from './state.js';
import { tick, pingRate, DEFAULT_SPEED_LIMIT } from './config.js';
import { mapInstance } from './map.js';
import { toggleDrivingHUD, updateDistance, onArrival, showAlert, updateETA, updateTrafficStatus, renderMainCarDebug } from './ui.js';
import { connectToSession, createSession, deleteSession, disconnectSession, sendLocationPing } from './api.js';
import { ensureSimulationCarsFeed, getLatestSimulationCars } from './debug-cars.js';
import { processRawRoute, projectPositionOntoRoute } from './utils.js';
import { advanceSpeedKmh, createDriverProfile, createMotionState, resolveTargetSpeedKmh } from './traffic-model.js';

let drivingWorker = null;
let pingElapsedMs = 0;
let lastTickAt = 0;

const FOLLOWING_TIME_SEC = 1.8;
const MIN_GAP_M = 7;
const MAX_TRACKED_GAP_M = 80;
const ROUTE_CAPTURE_M = 18;

function hasEdgeId(edgeId) {
    return edgeId !== null && edgeId !== undefined;
}

function computeTrafficStatus() {
    const ahead = (state.currentRoute || []).slice(state.currentRoadIndex, state.currentRoadIndex + 6);
    let slowEdges = 0;
    let worstRatio = 1;

    ahead.forEach((step) => {
        if (!hasEdgeId(step?.edge_id)) return;
        const base = step.speed_limit || 0;
        const recommended = state.recommendedSpeeds.get(step.edge_id);
        if (recommended == null || base <= 0) return;

        const ratio = recommended / base;
        if (ratio < 0.95) {
            slowEdges++;
            worstRatio = Math.min(worstRatio, ratio);
        }
    });

    if (slowEdges === 0) {
        if (state.navDebug?.congestion_ahead) {
            return {
                level: state.navDebug.congested_edges > 1 ? 'heavy' : 'congested',
                detail: `${state.navDebug.congested_edges} congested edge${state.navDebug.congested_edges === 1 ? '' : 's'} reported by server.`,
            };
        }
        return { level: 'normal', detail: 'No slowdowns detected on the active route.' };
    }

    if (worstRatio <= 0.6 || slowEdges >= 2) {
        return { level: 'heavy', detail: `${slowEdges} route edges have heavy slowdowns.` };
    }
    return { level: 'congested', detail: `${slowEdges} route edge has a measurable slowdown.` };
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
    document.getElementById('hud-src-name').textContent = formatAddress(state.source.name);
    document.getElementById('hud-dst-name').textContent = formatAddress(state.dest.name);
}

function calculateNewPosition(metersToMove, elapsedMs) {
    let remainingMeters = metersToMove;
    let remainingElapsedMs = elapsedMs;
    let pos = state.carPos;

    while (remainingMeters > 0 && state.currentRoadIndex < state.currentRoute.length) {
        const step = state.currentRoute[state.currentRoadIndex];
        const stepDist = step.base_length || 0.1; 
        const distanceLeftOnStep = stepDist - state.stepProgress;

        const startPos = [step.from_node[1], step.from_node[0]];
        const endPos = [step.to_node[1], step.to_node[0]];

        if (distanceLeftOnStep <= remainingMeters) {
            const spilloverMs = remainingMeters > 0
                ? remainingElapsedMs * ((remainingMeters - distanceLeftOnStep) / remainingMeters)
                : 0;
            const observedMs = Math.max(0, state.currentEdgeTimeMs - spilloverMs);
            remainingMeters -= distanceLeftOnStep;
            remainingElapsedMs = spilloverMs;
            if (hasEdgeId(step.edge_id) && observedMs > 0) {
                state.pendingEdgeEvents.push({
                    edge_id: step.edge_id,
                    observed_sec: observedMs / 1000,
                });
            }
            state.currentEdgeTimeMs = spilloverMs;
            state.currentRoadIndex++;
            state.stepProgress = 0;
            pos = endPos; 
        } else {
            state.stepProgress += remainingMeters;
            const fraction = state.stepProgress / stepDist;
            
            pos = [
                startPos[0] + (endPos[0] - startPos[0]) * fraction,
                startPos[1] + (endPos[1] - startPos[1]) * fraction
            ];
            remainingMeters = 0;
            remainingElapsedMs = 0;
        }
    }

    if (state.isDrifting) {
        const driftLatPerMs = 0.0000006; 
        const driftLngPerMs = 0.0000006;
        state.offRouteOffset[0] += driftLatPerMs * elapsedMs;
        state.offRouteOffset[1] += driftLngPerMs * elapsedMs;
    }

    return [
        pos[0] + state.offRouteOffset[0],
        pos[1] + state.offRouteOffset[1]
    ];
}

function limitMovementByTraffic(metersToMove, speedKmh) {
    if (!state.currentRouteObj || !state.carPos) {
        return metersToMove;
    }

    const myProgressM = Math.max(0, (state.currentRouteObj.distance || 0) - (state.distanceLeft || 0));
    const safetyGapM = Math.max(MIN_GAP_M, (speedKmh / 3.6) * FOLLOWING_TIME_SEC);
    let allowedMoveM = metersToMove;

    for (const car of state.simulationCars) {
        const projected = projectPositionOntoRoute(state.currentRouteObj, car.lat, car.lon);
        if (!projected || projected.offsetM > ROUTE_CAPTURE_M) {
            continue;
        }

        const otherProgressM = Math.max(0, (state.currentRouteObj.distance || 0) - projected.distanceLeft);
        const gapM = otherProgressM - myProgressM;
        if (gapM <= 0 || gapM > MAX_TRACKED_GAP_M) {
            continue;
        }

        allowedMoveM = Math.min(allowedMoveM, Math.max(0, gapM - safetyGapM));
    }

    return Math.max(0, allowedMoveM);
}

function getCurrentSpeedKmh() {
    if (state.currentRoadIndex >= state.currentRoute.length) {
        return DEFAULT_SPEED_LIMIT * 3.6;
    }
    const currentStep = state.currentRoute[state.currentRoadIndex];
    const recommendedSpeedKmh = hasEdgeId(currentStep.edge_id)
        ? state.recommendedSpeeds.get(currentStep.edge_id)
        : null;
    return recommendedSpeedKmh || currentStep.speed_limit || (DEFAULT_SPEED_LIMIT * 3.6);
}

function calculateBearing(currentPos) {

    if (state.currentRoadIndex >= state.currentRoute.length) return 0;
    
    const step = state.currentRoute[state.currentRoadIndex];
    const nextPos = [step.to_node[1], step.to_node[0]];
    
    return Math.atan2(nextPos[1] - currentPos[1], nextPos[0] - currentPos[0]) * 180 / Math.PI;
}

function handleLocationPing(pos, speedKmh) {
    if (pingElapsedMs >= pingRate) {
        const stepIndex = Math.min(state.currentRoadIndex + 1, state.currentRouteObj.steps.length - 1);
        sendLocationPing(pos[0], pos[1], speedKmh, stepIndex, state.pendingEdgeEvents);
        state.pendingEdgeEvents = [];
        pingElapsedMs %= pingRate;
    }
}

function ensureDrivingWorker() {
    if (drivingWorker) return drivingWorker;

    drivingWorker = new Worker('javascript/simulation-worker.js');
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
    if (state.currentRoute.length === 0 || state.isNavigating) return;
    state.recommendedSpeeds.clear();
    mapInstance.drawAlternatives([]);

    try {
        await ensureSimulationCarsFeed();
        const sessionId = await createSession(state.currentRouteObj.id);
        await connectToSession(sessionId, {
            onEtaUpdate: ({ eta_sec }) => {
                updateETA(eta_sec);
            },
            onReroute: (message) => {
                onReroute(message);
            },
            onSpeedUpdate: ({ edge_id, recommended_speed_kmh }) => {
                if (edge_id !== null && edge_id !== undefined) {
                    state.recommendedSpeeds.set(edge_id, recommended_speed_kmh);
                }
                refreshTrafficStatus();
            },
            onDebugUpdate: (debug) => {
                state.navDebug = debug;
                renderMainCarDebug(debug);
                refreshTrafficStatus();
            },
        });
        state.sessionId = sessionId;
    } catch (err) {
        console.error('Failed to start navigation session:', err);
        showAlert("Navigation failed", err.message);
        return;
    }

    setupDrivingUI();

    state.currentRoadIndex = 0;
    state.stepProgress = 0;
    state.isNavigating = true;
    state.distanceLeft = state.currentRouteObj.distance;
    state.pendingEdgeEvents = [];
    state.currentEdgeTimeMs = 0;
    state.driverProfile = createDriverProfile();
    state.motionState = createMotionState();
    state.navDebug = null;
    state.simulationCars = getLatestSimulationCars();
    renderMainCarDebug(null);
    pingElapsedMs = 0;
    lastTickAt = 0;

    const firstCoord = state.currentRoute[0].from_node;
    state.carPos = [firstCoord[1], firstCoord[0]];
    mapInstance.initCarMarker(state.carPos);

    ensureDrivingWorker().postMessage({ type: 'start', tickMs: tick });
}

function updateSimulation(elapsedMs) {
    if (state.currentRoadIndex >= state.currentRoute.length) {
        void stopDriving();
        onArrival(state.currentRouteObj.distance, state.currentRouteObj.dynamicETA);
        return;
    }

    const speedKmhValue = getCurrentSpeedKmh();
    pingElapsedMs += elapsedMs;
    state.currentEdgeTimeMs += elapsedMs;

    const activeEdgeId = state.currentRoute[state.currentRoadIndex]?.edge_id;
    const targetSpeedKmh = resolveTargetSpeedKmh(
        state.currentRoute[state.currentRoadIndex],
        hasEdgeId(activeEdgeId) ? state.recommendedSpeeds.get(activeEdgeId) : null,
        state.driverProfile,
    );
    state.motionState.speedKmh = advanceSpeedKmh(state.motionState.speedKmh || speedKmhValue, targetSpeedKmh, elapsedMs, state.driverProfile);
    state.simulationCars = getLatestSimulationCars();
    const unclampedSpeedKmh = state.motionState.speedKmh;
    const unclampedMetersThisTick = (unclampedSpeedKmh / 3.6) * (elapsedMs / 1000);
    const metersThisTick = limitMovementByTraffic(unclampedMetersThisTick, unclampedSpeedKmh);
    if (unclampedMetersThisTick > 0 && metersThisTick < unclampedMetersThisTick) {
        state.motionState.speedKmh = (metersThisTick / (elapsedMs / 1000)) * 3.6;
    }
    const speedKmh = Math.round(state.motionState.speedKmh);

    state.distanceLeft = Math.max(0, state.distanceLeft - metersThisTick);

    state.carPos = calculateNewPosition(metersThisTick, elapsedMs);
    const bearing = calculateBearing(state.carPos);

    mapInstance.updateCarPositionAndRotation([state.carPos[0], state.carPos[1]], bearing);

    handleLocationPing(state.carPos, speedKmh);

    updateDistance(state.distanceLeft, 'hud-distance-left');
    document.getElementById('hud-speed').textContent = `${speedKmh} km / h`;
}

export async function stopDriving() {
    state.isNavigating = false;
    if (drivingWorker) {
        drivingWorker.postMessage({ type: 'stop' });
    }
    pingElapsedMs = 0;
    lastTickAt = 0;

    // Hide driving HUD and drawings
    toggleDrivingHUD(false);
    mapInstance.removeCarMarker();

    // Show route and search panel
    if (state.currentRouteObj) {
        document.getElementById('route-panel').classList.remove('hidden');
    }
    document.getElementById('search-panel').classList.remove('hidden');

    disconnectSession();
    state.recommendedSpeeds.clear();
    state.pendingEdgeEvents = [];
    state.currentEdgeTimeMs = 0;
    state.driverProfile = null;
    state.motionState = null;
    state.navDebug = null;
    state.simulationCars = [];
    renderMainCarDebug(null);
    updateTrafficStatus('normal');

    const sessionId = state.sessionId;
    state.sessionId = null;
    if (sessionId) {
        try {
            await deleteSession(sessionId);
        } catch (err) {
            console.error('Failed to delete navigation session:', err);
        }
    }
}

export function onReroute(message) {

    state.offRouteOffset = [0, 0];
    state.isDrifting = false;

    const routeObj = processRawRoute(message.route);
    const oldRouteObj = state.currentRouteObj;

    const projected = state.carPos
        ? projectPositionOntoRoute(routeObj, state.carPos[0], state.carPos[1])
        : null;
    const reason = message.reroute_reason || 'traffic';
    const oldEtaSec = message.old_eta_sec;
    const newEtaSec = message.new_eta_sec;
    const etaGainMin = oldEtaSec && newEtaSec ? Math.max(0, Math.round((oldEtaSec - newEtaSec) / 60)) : 0;

    if (state.navDebug) {
        state.navDebug.last_reroute_reason = reason;
        state.navDebug.last_reroute_at_unix_ms = Date.now();
        renderMainCarDebug(state.navDebug);
    }

    if (reason === 'off_route') {
        const dist = state.navDebug?.off_route_distance_m;
        const distText = typeof dist === 'number' ? `${dist.toFixed(1)} m away from the expected path.` : 'Vehicle left the expected path.';
        showAlert('Off-route reroute', distText);
    } else {
        const gainText = etaGainMin > 0 ? `New path saves about ${etaGainMin} min.` : 'New path is faster under current traffic.';
        showAlert('Traffic reroute', gainText);
    }
    state.recommendedSpeeds.clear();
    state.currentEdgeTimeMs = 0;

    state.currentRouteObj = routeObj;
    state.currentRoute = routeObj.legs; 
    updateETA(message.new_eta_sec ?? routeObj.dynamicETA);

    if (state.isNavigating) {
        state.pendingEdgeEvents = [];
        if (projected) {
            state.currentRoadIndex = projected.roadIndex;
            state.stepProgress = projected.stepProgress;
            state.distanceLeft = projected.distanceLeft;
            state.carPos = [projected.lat, projected.lng];
        } else {
            state.currentRoadIndex = 0;
            state.stepProgress = 0;
            state.distanceLeft = routeObj.distance;
        }
        if (!state.driverProfile) {
            state.driverProfile = createDriverProfile();
        }
        if (!state.motionState) {
            state.motionState = createMotionState();
        }
        updateDistance(state.distanceLeft, 'hud-distance-left');
    } else {
        updateDistance(routeObj.distance, 'hud-distance-left');
    }

    if (oldRouteObj) {
        mapInstance.drawAlternatives([oldRouteObj]);
    }
    
    mapInstance.drawRoute(routeObj.pathCoords, state.source, state.dest);
    mapInstance.setRouteInspectorRoute(routeObj);
    if (state.isNavigating && state.carPos) {
        const bearing = calculateBearing(state.carPos);
        mapInstance.updateCarPositionAndRotation([state.carPos[0], state.carPos[1]], bearing);
        const stepIndex = Math.min(state.currentRoadIndex + 1, state.currentRouteObj.steps.length - 1);
        sendLocationPing(state.carPos[0], state.carPos[1], Math.round(getCurrentSpeedKmh()), stepIndex, []);
    }
    updateTrafficStatus('normal');
}
