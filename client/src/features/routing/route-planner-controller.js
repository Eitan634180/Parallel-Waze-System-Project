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
import { CSS_CLASSES, DOM_IDS, PANEL_TEXT, TRAFFIC_LEVELS, UI_KEYS } from '../../ui/ui-constants.js';

const FIRST_ROUTE_INDEX = 0;
const ROUTE_OPTION_SELECTOR = '.route-option-card';
const ROUTE_LOG_MESSAGES = {
    calculateFailed: 'Route calculation failed',
};

export function initializeRoutePlanner() {
    const invalidateRoutingState = (key) => {
        if (state.routing[key] === null) return;
        state.routing[key] = null;
        clearCurrentRoute();
        syncNavigateButton();
    };

    setupSearchInput(
        DOM_IDS.sourceInput,
        DOM_IDS.sourceSuggestions,
        (location) => {
            state.routing.source = location;
            syncNavigateButton();
        },
        () => invalidateRoutingState('source'),
    );

    setupSearchInput(
        DOM_IDS.destInput,
        DOM_IDS.destSuggestions,
        (location) => {
            state.routing.dest = location;
            syncNavigateButton();
        },
        () => invalidateRoutingState('dest'),
    );

    document.getElementById(DOM_IDS.navigateButton).addEventListener('click', handleCalculateRoute);
    document.getElementById(DOM_IDS.closeRoute).addEventListener('click', closeRoutePanel);
}

function syncNavigateButton() {
    document.getElementById(DOM_IDS.navigateButton).disabled = !(state.routing.source && state.routing.dest);
}

function clearCurrentRoute() {
    state.routing.allRoutes = [];
    state.routing.currentIndex = 0;
    state.routing.activeObj = null;
    state.routing.activeLegs = [];
    state.sim.recommendedSpeeds.clear();

    document.getElementById(DOM_IDS.routePanel).classList.add(CSS_CLASSES.hidden);
    updateETA(null);
    updateDistance(0);
    updateTrafficStatus(TRAFFIC_LEVELS.normal);

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
        document.getElementById(DOM_IDS.routePanel).classList.remove(CSS_CLASSES.hidden);
        renderRouteOptions(state.routing.allRoutes, handleRouteSelection);
        handleRouteSelection(FIRST_ROUTE_INDEX);
        mapInstance.drawEndpointMarkers(state.routing.source, state.routing.dest);
    } catch (err) {
        console.error(ROUTE_LOG_MESSAGES.calculateFailed, err);
        showAlert(PANEL_TEXT.routeCalculationFailed, err.message);
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

    document.querySelectorAll(ROUTE_OPTION_SELECTOR).forEach((element) => element.classList.remove(CSS_CLASSES.active));
    document.getElementById(`${UI_KEYS.routeOptionIdPrefix}${index}`)?.classList.add(CSS_CLASSES.active);

    updateETA(route.dynamicETA);
    updateDistance(route.distance);

    const traffic = getRouteTrafficStatus(route);
    updateTrafficStatus(traffic.level, traffic.detail);

    mapInstance.drawRoute(route.pathCoords, state.routing.source, state.routing.dest);
    mapInstance.drawAlternatives(state.routing.allRoutes.filter((_, routeIndex) => routeIndex !== index));
}

function closeRoutePanel() {
    document.getElementById(DOM_IDS.routePanel).classList.add(CSS_CLASSES.hidden);
    state.debug.inspectorEnabled = false;
    setRouteInspectorButtonState(false);
    renderEdgeDebugInfo(null);
    mapInstance.setRouteInspectorEnabled(false);
    mapInstance.clearRouteLayers();
}
