import { mapInstance } from '../ui/map/map-manager.js';
import { setupDebugTools } from '../features/debug/debug-tools-controller.js';
import { initializeRoutePlanner } from '../features/routing/route-planner-controller.js';
import { startDriving, stopDriving } from '../features/driving/driving-controller.js';
import { fetchSystemInfo } from '../services/rest/navigation-api.js';

const BOOTSTRAP_LOG_MESSAGES = {
    systemInfoUnavailable: 'Bootstrap could not load system info; using default map center',
};

document.addEventListener('DOMContentLoaded', async () => {
    let systemInfo = null;
    try {
        systemInfo = await fetchSystemInfo();
    } catch (err) {
        console.warn(BOOTSTRAP_LOG_MESSAGES.systemInfoUnavailable, err);
    }

    mapInstance.initMap('map', systemInfo);
    initializeRoutePlanner();
    setupDebugTools();

    document.getElementById('start-drive-btn').addEventListener('click', startDriving);
    document.getElementById('stop-drive-btn').addEventListener('click', stopDriving);
});
