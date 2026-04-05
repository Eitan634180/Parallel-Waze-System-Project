import { state } from '../../app/app-state.js';
import { mapInstance } from '../../ui/map/map-manager.js';
import { showAlert } from '../../ui/panels/alerts-panel.js';
import { renderMainCarDebug } from '../../ui/panels/debug-panel.js';
import { updateDistance, updateETA } from '../../ui/panels/hud-panel.js';
import { sendLocationPing } from '../../services/ws/socket-client.js';
import { processRawRoute, projectPositionOntoRoute } from '../routing/route-utils.js';
import { createDriverProfile, createMotionState } from './traffic-model.js';
import { DOM_IDS, PANEL_TEXT } from '../../ui/ui-constants.js';

const SECONDS_PER_MINUTE = 60;
const DEFAULT_REROUTE_REASON = 'traffic';
const STEP_INDEX_OFFSET = 1;
const ZERO_OFFSET = Object.freeze([0, 0]);

function createZeroOffset() {
    return [...ZERO_OFFSET];
}

export function handleDrivingReroute(message, context) {
    resetOffRouteState();

    const routeObj = processRawRoute(message.route);
    const previousRoute = state.routing.activeObj;
    const projectedPosition = state.drive.carPos
        ? projectPositionOntoRoute(routeObj, state.drive.carPos[0], state.drive.carPos[1])
        : null;
    const reason = message.reroute_reason || DEFAULT_REROUTE_REASON;

    syncDebugRerouteState(reason);
    showRerouteAlert(reason, message.old_eta_sec, message.new_eta_sec);

    state.sim.recommendedSpeeds.clear();
    state.sim.currentEdgeTimeMs = 0;
    state.routing.activeObj = routeObj;
    state.routing.activeLegs = routeObj.legs;

    updateETA(message.new_eta_sec ?? routeObj.dynamicETA);
    syncDrivingProgressAfterReroute(routeObj, projectedPosition);

    if (previousRoute) {
        mapInstance.drawAlternatives([previousRoute]);
    }

    mapInstance.drawRoute(routeObj.pathCoords, state.routing.source, state.routing.dest);
    mapInstance.setRouteInspectorRoute(routeObj);

    if (state.drive.isActive && state.drive.carPos) {
        const bearing = context.getBearing(state.drive.carPos);
        mapInstance.updateCarPositionAndRotation([state.drive.carPos[0], state.drive.carPos[1]], bearing);
        const stepIndex = Math.min(state.drive.currentRoadIndex + STEP_INDEX_OFFSET, state.routing.activeObj.steps.length - STEP_INDEX_OFFSET);
        sendLocationPing(state.drive.carPos[0], state.drive.carPos[1], Math.round(context.getCurrentSpeedKmh()), stepIndex, []);
    }

    context.refreshTrafficStatus();
}

function resetOffRouteState() {
    state.drive.offRouteOffset = createZeroOffset();
    state.drive.isDrifting = false;
}

function syncDebugRerouteState(reason) {
    if (!state.debug.serverData) {
        return;
    }

    state.debug.serverData.last_reroute_reason = reason;
    state.debug.serverData.last_reroute_at_unix_ms = Date.now();
    renderMainCarDebug(state.debug.serverData);
}

function showRerouteAlert(reason, oldEtaSec, newEtaSec) {
    if (reason === 'off_route') {
        const distance = state.debug.serverData?.off_route_distance_m;
        const detail = typeof distance === 'number'
            ? `${distance.toFixed(1)} m away from the expected path.`
            : PANEL_TEXT.offRouteDescription;
        showAlert(PANEL_TEXT.offRouteTitle, detail);
        return;
    }

    showAlert(PANEL_TEXT.trafficRerouteTitle, formatRerouteGainText(oldEtaSec, newEtaSec));
}

function syncDrivingProgressAfterReroute(routeObj, projectedPosition) {
    if (state.drive.isActive) {
        state.sim.pendingEdgeEvents = [];
        if (projectedPosition) {
            state.drive.currentRoadIndex = projectedPosition.roadIndex;
            state.drive.stepProgress = projectedPosition.stepProgress;
            state.drive.distanceLeft = projectedPosition.distanceLeft;
            state.drive.carPos = [projectedPosition.lat, projectedPosition.lng];
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
        updateDistance(state.drive.distanceLeft, DOM_IDS.hudDistanceLeft);
        return;
    }

    updateDistance(routeObj.distance, DOM_IDS.hudDistanceLeft);
}

function formatRerouteGainText(oldEtaSec, newEtaSec) {
    if (oldEtaSec == null || newEtaSec == null) {
        return PANEL_TEXT.routeUpdated;
    }

    const etaGainSec = Math.max(0, oldEtaSec - newEtaSec);
    if (etaGainSec >= SECONDS_PER_MINUTE) {
        return `New path saves about ${Math.round(etaGainSec / SECONDS_PER_MINUTE)} min.`;
    }
    if (etaGainSec > 0) {
        return `New path saves about ${Math.round(etaGainSec)} sec.`;
    }
    return PANEL_TEXT.routeUpdated;
}
