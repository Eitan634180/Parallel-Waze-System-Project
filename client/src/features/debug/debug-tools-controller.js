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
    syncStoredDebugCars,
} from './debug-cars-service.js';
import { DOM_IDS, PANEL_TEXT } from '../../ui/ui-constants.js';
import { DEBUG_CARS } from './config.js';

const ZERO_OFFSET = Object.freeze([0, 0]);
const DEBUG_LOG_MESSAGES = {
    addCarsFailed: 'Adding debug cars failed',
    randomTrafficFailed: 'Spawning random traffic failed',
};

function createZeroOffset() {
    return [...ZERO_OFFSET];
}

function readDebugCarCount(input) {
    const requestedCount = Number(input.value);
    if (!Number.isFinite(requestedCount)) {
        return DEBUG_CARS.defaultCount;
    }
    return Math.min(DEBUG_CARS.maximumCount, Math.max(DEBUG_CARS.minimumCount, requestedCount));
}

export function setupDebugTools() {
    let isOpen = false;
    const toggleDebugCarsButton = document.getElementById(DOM_IDS.toggleDebugCarsButton);

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
        toggleDebugCarsButton.textContent = state.debug.carsVisible ? PANEL_TEXT.hideTestCars : PANEL_TEXT.showTestCars;
        toggleDebugCarsButton.classList.toggle('active', !state.debug.carsVisible);
        if (state.debug.carsVisible) {
            syncStoredDebugCars();
        }
        mapInstance.setDebugCarsVisible(state.debug.carsVisible);
    };

    document.getElementById(DOM_IDS.debugToggleButton).addEventListener('click', () => {
        isOpen = !isOpen;
        toggleDebugPanel(isOpen);
    });

    document.getElementById(DOM_IDS.closeDebug).addEventListener('click', () => {
        isOpen = false;
        toggleDebugPanel(false);
    });

    document.getElementById(DOM_IDS.toggleRouteInspectorButton).addEventListener('click', () => {
        state.debug.inspectorEnabled = !state.debug.inspectorEnabled;
        syncInspector();
    });

    toggleDebugCarsButton.addEventListener('click', () => {
        state.debug.carsVisible = !state.debug.carsVisible;
        syncDebugCarsVisibility();
    });

    setDebugCarCallback(renderDebugCarStatus);
    renderMainCarDebug(null);

    const debugCarCountInput = document.getElementById(DOM_IDS.debugCarCount);
    debugCarCountInput.min = String(DEBUG_CARS.minimumCount);
    debugCarCountInput.max = String(DEBUG_CARS.maximumCount);
    debugCarCountInput.value = String(DEBUG_CARS.defaultCount);

    document.getElementById(DOM_IDS.debugAddCarButton).addEventListener('click', async () => {
        try {
            const count = readDebugCarCount(debugCarCountInput);
            await spawnDebugCars(state.routing.activeObj, count, state.drive.currentRoadIndex || 0);
        } catch (err) {
            console.error(DEBUG_LOG_MESSAGES.addCarsFailed, err);
            showAlert(PANEL_TEXT.debugCarsFailed, err.message);
        }
    });

    document.getElementById(DOM_IDS.debugRandomTrafficButton).addEventListener('click', async () => {
        try {
            const count = readDebugCarCount(debugCarCountInput);
            await spawnRandomDebugCars(count);
        } catch (err) {
            console.error(DEBUG_LOG_MESSAGES.randomTrafficFailed, err);
            showAlert(PANEL_TEXT.randomTrafficFailed, err.message);
        }
    });

    document.getElementById(DOM_IDS.debugClearCarsButton).addEventListener('click', () => {
        void clearDebugCars();
    });

    document.getElementById(DOM_IDS.debugDriftButton).addEventListener('click', () => {
        state.drive.isDrifting = !state.drive.isDrifting;
        if (!state.drive.isDrifting) {
            state.drive.offRouteOffset = createZeroOffset();
        }
    });

    syncInspector();
    syncDebugCarsVisibility();
}
