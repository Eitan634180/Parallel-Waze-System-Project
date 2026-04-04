import { state } from '../../app/app-state.js';
import { mapInstance } from '../../ui/map/map-manager.js';
import { fetchRoute } from '../../services/rest/navigation-api.js';
import { setupSearchInput } from './search-input-controller.js';
import { toggleLoadingState, showAlert } from '../../ui/panels/alerts-panel.js';
import { renderRouteOptions } from '../../ui/panels/route-options-panel.js';
import { updateETA, updateDistance, updateTrafficStatus } from '../../ui/panels/hud-panel.js';
import { renderEdgeDebugInfo, setRouteInspectorButtonState } from '../../ui/panels/debug-panel.js';
import { getRouteTrafficStatus } from '../driving/traffic-evaluator.js';
import { processRawRoute } from './route-utils.js';

export function initializeRoutePlanner() {
    const invalidateRoutingState = (key) => {
        if (state.routing[key] === null) return;
        state.routing[key] = null;
        clearCurrentRoute();
        syncNavigateButton();
    };

    setupSearchInput(
        'source-input',
        'source-suggestions',
        (location) => {
            state.routing.source = location;
            syncNavigateButton();
        },
        () => invalidateRoutingState('source'),
    );

    setupSearchInput(
        'dest-input',
        'dest-suggestions',
        (location) => {
            state.routing.dest = location;
            syncNavigateButton();
        },
        () => invalidateRoutingState('dest'),
    );

    document.getElementById('navigate-btn').addEventListener('click', handleCalculateRoute);
    document.getElementById('close-route').addEventListener('click', closeRoutePanel);
}

function syncNavigateButton() {
    document.getElementById('navigate-btn').disabled = !(state.routing.source && state.routing.dest);
}

function clearCurrentRoute() {
    state.routing.allRoutes = [];
    state.routing.currentIndex = 0;
    state.routing.activeObj = null;
    state.routing.activeLegs = [];
    state.sim.recommendedSpeeds.clear();

    document.getElementById('route-panel').classList.add('hidden');
    updateETA(null);
    updateDistance(0);
    updateTrafficStatus('normal');

    state.debug.inspectorEnabled = false;
    setRouteInspectorButtonState(false);
    renderEdgeDebugInfo(null);
    mapInstance.setRouteInspectorEnabled(false);
    mapInstance.setRouteInspectorRoute(null);
    mapInstance.clearRouteLayers();
}

async function handleCalculateRoute() {
    if (!state.routing.source || !state.routing.dest) return;

    toggleLoadingState(true);
    try {
        const rawRoutes = await fetchRoute(
            state.routing.source.lat,
            state.routing.source.lng,
            state.routing.dest.lat,
            state.routing.dest.lng,
        );

        state.routing.allRoutes = rawRoutes.map((route) => processRawRoute(route));
        document.getElementById('route-panel').classList.remove('hidden');
        renderRouteOptions(state.routing.allRoutes, handleRouteSelection);
        handleRouteSelection(0);
        mapInstance.drawEndpointMarkers(state.routing.source, state.routing.dest);
    } catch (err) {
        console.error('Route error:', err);
        showAlert('Route calculation failed', err.message);
    } finally {
        toggleLoadingState(false);
    }
}

function handleRouteSelection(index) {
    const route = state.routing.allRoutes[index];
    state.routing.activeObj = route;
    state.routing.activeLegs = route.legs;
    state.routing.currentIndex = index;
    state.sim.recommendedSpeeds.clear();

    mapInstance.setRouteInspectorRoute(route);
    renderEdgeDebugInfo(null);

    document.querySelectorAll('.route-option-card').forEach((element) => element.classList.remove('active'));
    document.getElementById(`route-option-${index}`)?.classList.add('active');

    updateETA(route.dynamicETA);
    updateDistance(route.distance);

    const traffic = getRouteTrafficStatus(route);
    updateTrafficStatus(traffic.level, traffic.detail);

    mapInstance.drawRoute(route.pathCoords, state.routing.source, state.routing.dest);
    mapInstance.drawAlternatives(state.routing.allRoutes.filter((_, routeIndex) => routeIndex !== index));
}

function closeRoutePanel() {
    document.getElementById('route-panel').classList.add('hidden');
    state.debug.inspectorEnabled = false;
    setRouteInspectorButtonState(false);
    renderEdgeDebugInfo(null);
    mapInstance.setRouteInspectorEnabled(false);
    mapInstance.clearRouteLayers();
}
