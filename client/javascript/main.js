import { state } from './state.js';
import { mapInstance } from './map.js';
import { fetchRoute } from './api.js';
import { setupSearchInput, toggleLoadingState, showAlert, renderRouteOptions, updateETA, updateDistance, updateTrafficStatus, toggleDebugPanel, setRouteInspectorButtonState, renderEdgeDebugInfo, renderDebugCarStatus, renderMainCarDebug } from './ui.js';
import { startDriving, stopDriving } from './simulation.js';
import { spawnDebugCars, spawnRandomDebugCars, clearDebugCars, subscribeDebugCars } from './debug-cars.js';
import { processRawRoute } from './utils.js';

async function handleCalculateRoute() {

    if (!state.source || !state.dest) return;
    toggleLoadingState(true);

    try {

        // Fetch and parse routes
        const rawRoutes = await fetchRoute(
            state.source.lat, state.source.lng, 
            state.dest.lat, state.dest.lng
        );
        state.allRoutes = rawRoutes.map(route => processRawRoute(route));
        
        // Display on route panel
        document.getElementById('route-panel').classList.remove('hidden');
        renderRouteOptions(state.allRoutes, handleRouteSelection); 

        // Default to first route
        handleRouteSelection(0); 
        mapInstance.drawEndpointMarkers(state.source, state.dest);
        
    } catch (err) {
        console.error('Route error:', err);
        showAlert("Route calculation failed", err.message);
    } finally {
        toggleLoadingState(false);
    }
}

function handleRouteSelection(index) {

    // Set state
    const route = state.allRoutes[index];
    state.currentRouteObj = route;
    state.currentRoute = route.legs; 
    state.currentRouteIndex = index; 
    state.recommendedSpeeds.clear();
    mapInstance.setRouteInspectorRoute(route);
    renderEdgeDebugInfo(null);

    document.querySelectorAll('.route-option-card').forEach(el => el.classList.remove('active'));
    const selectedCard = document.getElementById(`route-option-${index}`);
    if (selectedCard) selectedCard.classList.add('active');

    // Update UI
    updateETA(route.dynamicETA);
    updateDistance(route.distance);
    updateTrafficStatus('normal');

    // Update map
    mapInstance.drawRoute(route.pathCoords, state.source, state.dest)
    mapInstance.drawAlternatives(state.allRoutes.filter((_, i) => i !== index));
}

function setupDebugTools() {
    let isOpen = false;
    const toggleDebugCarsButton = document.getElementById('toggle-debug-cars-btn');

    const syncInspector = () => {
        setRouteInspectorButtonState(state.debugRouteInspector);
        mapInstance.setRouteInspectorEnabled(
            state.debugRouteInspector,
            (edge) => renderEdgeDebugInfo(edge),
        );
        if (state.currentRouteObj) {
            mapInstance.setRouteInspectorRoute(state.currentRouteObj);
        }
        if (!state.debugRouteInspector) {
            renderEdgeDebugInfo(null);
        }
    };

    const syncDebugCarsVisibility = () => {
        toggleDebugCarsButton.textContent = state.debugCarsVisible ? 'Hide Test Cars' : 'Show Test Cars';
        toggleDebugCarsButton.classList.toggle('active', !state.debugCarsVisible);
        mapInstance.setDebugCarsVisible(state.debugCarsVisible);
    };

    document.getElementById('debug-toggle-btn').addEventListener('click', () => {
        isOpen = !isOpen;
        toggleDebugPanel(isOpen);
    });

    document.getElementById('close-debug').addEventListener('click', () => {
        isOpen = false;
        toggleDebugPanel(false);
    });

    document.getElementById('toggle-route-inspector-btn').addEventListener('click', () => {
        state.debugRouteInspector = !state.debugRouteInspector;
        syncInspector();
    });

    toggleDebugCarsButton.addEventListener('click', () => {
        state.debugCarsVisible = !state.debugCarsVisible;
        syncDebugCarsVisibility();
    });

    subscribeDebugCars(renderDebugCarStatus);
    renderMainCarDebug(null);

    document.getElementById('debug-add-car-btn').addEventListener('click', async () => {
        try {
            const count = Math.max(1, Number(document.getElementById('debug-car-count').value) || 1);
            
            await spawnDebugCars(
                state.currentRouteObj, 
                count, 
                [],
                state.currentRoadIndex || 0
            );
            
        } catch (err) {
            console.error('Failed to add debug cars:', err);
            showAlert('Debug cars failed', err.message);
        }
    });

    document.getElementById('debug-random-traffic-btn').addEventListener('click', async () => {
        try {
            const count = Math.max(1, Number(document.getElementById('debug-car-count').value) || 1);
            await spawnRandomDebugCars(count);
        } catch (err) {
            console.error('Failed to spawn random traffic:', err);
            showAlert('Random traffic failed', err.message);
        }
    });

    document.getElementById('debug-clear-cars-btn').addEventListener('click', () => {
        void clearDebugCars();
    });

    document.getElementById('debug-drift-btn').addEventListener('click', () => {
        state.isDrifting = !state.isDrifting;
        
        if (!state.isDrifting) {
            state.offRouteOffset = [0, 0]; 
        }
        
        console.log("Spoof Drifting is now:", state.isDrifting ? "ACTIVE" : "OFF");
    });

    syncInspector();
    syncDebugCarsVisibility();
}

document.addEventListener('DOMContentLoaded', () => {

    mapInstance.initMap();
    setupDebugTools();

    setupSearchInput('source-input', 'source-suggestions', (loc) => {
        state.source = loc;
        document.getElementById('navigate-btn').disabled = !(state.source && state.dest);
    });

    setupSearchInput('dest-input', 'dest-suggestions', (loc) => {
        state.dest = loc;
        document.getElementById('navigate-btn').disabled = !(state.source && state.dest);
    });

    document.getElementById('navigate-btn').addEventListener('click', handleCalculateRoute);
    
    document.getElementById('close-route').addEventListener('click', () => {
        document.getElementById('route-panel').classList.add('hidden');
        state.debugRouteInspector = false;
        setRouteInspectorButtonState(false);
        renderEdgeDebugInfo(null);
        mapInstance.setRouteInspectorEnabled(false);
        mapInstance.clearRouteLayers();
    });

    document.getElementById('start-drive-btn').addEventListener('click', startDriving);
    document.getElementById('stop-drive-btn').addEventListener('click', stopDriving);
});
