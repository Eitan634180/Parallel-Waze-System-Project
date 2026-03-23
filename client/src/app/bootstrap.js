import { mapInstance } from '../ui/map/map-manager.js';
import { setupDebugTools } from '../features/debug/debug-tools-controller.js';
import { initializeRoutePlanner } from '../features/routing/route-planner-controller.js';
import { startDriving, stopDriving } from '../features/driving/driving-controller.js';

document.addEventListener('DOMContentLoaded', () => {
    mapInstance.initMap();
    initializeRoutePlanner();
    setupDebugTools();

    document.getElementById('start-drive-btn').addEventListener('click', startDriving);
    document.getElementById('stop-drive-btn').addEventListener('click', stopDriving);
});
