import { state } from '../../app/app-state.js';
import { mapInstance } from '../../ui/map/map-manager.js';
import {
    toggleDebugPanel,
    setRouteInspectorButtonState,
    renderEdgeDebugInfo,
    renderDebugCarStatus,
    renderMainCarDebug,
} from '../../ui/panels/debug-panel.js';
import { showAlert } from '../../ui/panels/alerts-panel.js';
import {
    spawnDebugCars,
    spawnRandomDebugCars,
    clearDebugCars,
    setDebugCarCallback,
} from './debug-cars-service.js';

export function setupDebugTools() {
    let isOpen = false;
    const toggleDebugCarsButton = document.getElementById('toggle-debug-cars-btn');

    const syncInspector = () => {
        setRouteInspectorButtonState(state.debug.inspectorEnabled);
        mapInstance.setRouteInspectorEnabled(
            state.debug.inspectorEnabled,
            (edge) => renderEdgeDebugInfo(edge),
        );
        if (state.routing.activeObj) {
            mapInstance.setRouteInspectorRoute(state.routing.activeObj);
        }
        if (!state.debug.inspectorEnabled) {
            renderEdgeDebugInfo(null);
        }
    };

    const syncDebugCarsVisibility = () => {
        toggleDebugCarsButton.textContent = state.debug.carsVisible ? 'Hide Test Cars' : 'Show Test Cars';
        toggleDebugCarsButton.classList.toggle('active', !state.debug.carsVisible);
        mapInstance.setDebugCarsVisible(state.debug.carsVisible);
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
        state.debug.inspectorEnabled = !state.debug.inspectorEnabled;
        syncInspector();
    });

    toggleDebugCarsButton.addEventListener('click', () => {
        state.debug.carsVisible = !state.debug.carsVisible;
        syncDebugCarsVisibility();
    });

    setDebugCarCallback(renderDebugCarStatus);
    renderMainCarDebug(null);

    document.getElementById('debug-add-car-btn').addEventListener('click', async () => {
        try {
            const count = Math.max(1, Number(document.getElementById('debug-car-count').value) || 1);
            await spawnDebugCars(state.routing.activeObj, count, state.drive.currentRoadIndex || 0);
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
        state.drive.isDrifting = !state.drive.isDrifting;
        if (!state.drive.isDrifting) {
            state.drive.offRouteOffset = [0, 0]; 
        }
        console.log("Spoof Drifting is now:", state.drive.isDrifting ? "ACTIVE" : "OFF");
    });

    syncInspector();
    syncDebugCarsVisibility();
}
