import { state } from './app-state.js';
import { loadRoutePlan, selectRoute } from '../features/routing/route-planner-controller.js';
import { startDriving, stopDriving } from '../features/driving/driving-controller.js';

const LOADBOT_QUERY_KEY = 'loadbot';

function isLoadbotEnabled() {
    return new URLSearchParams(window.location.search).has(LOADBOT_QUERY_KEY);
}

function snapshot() {
    return {
        isDriving: state.drive.isActive,
        sessionId: state.drive.sessionId,
        currentRouteId: state.routing.activeObj?.id || null,
        currentRouteIndex: state.routing.currentIndex,
        currentRoadIndex: state.drive.currentRoadIndex,
        distanceLeft: state.drive.distanceLeft,
        carPos: state.drive.carPos,
        debug: state.debug.serverData,
    };
}

async function planTrip({ source, dest, routeIndex = 0 }) {
    const selected = await loadRoutePlan(source, dest);
    if (routeIndex > 0) {
        selectRoute(routeIndex);
    }
    return {
        planned: Boolean(selected),
        ...snapshot(),
    };
}

async function driveTrip({ source, dest, routeIndex = 0 }) {
    await planTrip({ source, dest, routeIndex });
    await startDriving();
    return snapshot();
}

export function registerLoadbotBridge() {
    if (!isLoadbotEnabled()) {
        return;
    }

    document.body.dataset.loadbot = '1';
    window.__loadbot = {
        planTrip,
        driveTrip,
        startDriving,
        stopDriving,
        state: snapshot,
    };
}
