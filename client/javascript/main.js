import { state } from './core/state.js';
import { mapInstance } from './ui/map.js';
import { fetchRoute } from './api/api-rest.js';
import { setupSearchInput } from './ui/ui-search.js';
import { toggleLoadingState, showAlert } from './ui/ui-alerts.js';
import { renderRouteOptions } from './ui/ui-routes.js';
import { updateETA, updateDistance, updateTrafficStatus } from './ui/ui-hud.js';
import { renderEdgeDebugInfo, setRouteInspectorButtonState } from './ui/ui-debug.js';
import { setupDebugTools } from './ui/debug-controller.js';
import { startDriving, stopDriving } from './simulation/driving-controller.js';
import { getRouteTrafficStatus } from './simulation/traffic-evaluator.js';
import { processRawRoute } from './utils/utils.js';

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
        const rawRoutes = await fetchRoute(state.routing.source.lat, state.routing.source.lng, state.routing.dest.lat, state.routing.dest.lng);
        state.routing.allRoutes = rawRoutes.map(route => processRawRoute(route));
        
        document.getElementById('route-panel').classList.remove('hidden');
        renderRouteOptions(state.routing.allRoutes, handleRouteSelection); 

        handleRouteSelection(0); 
        mapInstance.drawEndpointMarkers(state.routing.source, state.routing.dest);
        
    } catch (err) {
        console.error('Route error:', err);
        showAlert("Route calculation failed", err.message);
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

    document.querySelectorAll('.route-option-card').forEach(el => el.classList.remove('active'));
    const selectedCard = document.getElementById(`route-option-${index}`);
    if (selectedCard) selectedCard.classList.add('active');

    updateETA(route.dynamicETA);
    updateDistance(route.distance);
    const traffic = getRouteTrafficStatus(route);
    updateTrafficStatus(traffic.level, traffic.detail);

    mapInstance.drawRoute(route.pathCoords, state.routing.source, state.routing.dest)
    mapInstance.drawAlternatives(state.routing.allRoutes.filter((_, i) => i !== index));
}

document.addEventListener('DOMContentLoaded', () => {
    mapInstance.initMap();
    setupDebugTools();

    const invalidateRoutingState = (key) => {
        if (state.routing[key] === null) return;
        state.routing[key] = null;
        clearCurrentRoute();
        syncNavigateButton();
    };

    setupSearchInput('source-input', 'source-suggestions', (loc) => {
        state.routing.source = loc;
        syncNavigateButton();
    }, () => invalidateRoutingState('source'));

    setupSearchInput('dest-input', 'dest-suggestions', (loc) => {
        state.routing.dest = loc;
        syncNavigateButton();
    }, () => invalidateRoutingState('dest'));

    document.getElementById('navigate-btn').addEventListener('click', handleCalculateRoute);
    
    document.getElementById('close-route').addEventListener('click', () => {
        document.getElementById('route-panel').classList.add('hidden');
        state.debug.inspectorEnabled = false;
        setRouteInspectorButtonState(false);
        renderEdgeDebugInfo(null);
        mapInstance.setRouteInspectorEnabled(false);
        mapInstance.clearRouteLayers();
    });

    document.getElementById('start-drive-btn').addEventListener('click', startDriving);
    document.getElementById('stop-drive-btn').addEventListener('click', stopDriving);
});
