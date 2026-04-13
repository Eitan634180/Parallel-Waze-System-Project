import { formatDuration, getArrivalTime } from '../../utils/formatters.js';
import { CSS_CLASSES, DOM_IDS, PANEL_TEXT, TRAFFIC_LEVELS } from '../ui-constants.js';

const DISPLAY_STYLES = {
    flex: 'flex',
    none: 'none',
};

const DISTANCE_CONVERSION = {
    metersPerKilometer: 1000,
};

export function updateETA(seconds) {
    const durationStr = formatDuration(seconds);
    document.getElementById(DOM_IDS.etaValue).textContent = durationStr;
    document.getElementById(DOM_IDS.hudEta).textContent = durationStr;
    document.getElementById(DOM_IDS.hudArrivalTime).textContent = getArrivalTime(seconds);
}

export function updateDistance(meters, elementId = DOM_IDS.distanceValue) {
    const km = (meters / DISTANCE_CONVERSION.metersPerKilometer).toFixed(1);
    document.getElementById(elementId).textContent = `${km} km`;
    if (km >= 100) document.getElementById(elementId).style.fontSize = '14px';
}

export function updateTrafficStatus(status = TRAFFIC_LEVELS.normal, detail = '') {
    const el = document.getElementById(DOM_IDS.trafficStatus);
    if (!el) return;

    if (typeof status === 'boolean') status = status ? TRAFFIC_LEVELS.congested : TRAFFIC_LEVELS.normal;

    if (status === TRAFFIC_LEVELS.heavy) {
        el.textContent = PANEL_TEXT.heavy;
        el.className = 'stat-value heavy';
    } else if (status === TRAFFIC_LEVELS.congested) {
        el.textContent = PANEL_TEXT.congested;
        el.className = 'stat-value congested';
    } else {
        el.textContent = PANEL_TEXT.normal;
        el.className = 'stat-value';
    }
    el.title = detail || '';
}

export function toggleDrivingHUD(isDriving) {
    const hud = document.getElementById(DOM_IDS.drivingHud);
    const sidebar = document.getElementById(DOM_IDS.sidebar);
    const stopBtn = document.getElementById(DOM_IDS.stopDriveButton);
    const startBtn = document.getElementById(DOM_IDS.startDriveButton);

    if (isDriving) {
        hud.classList.remove(CSS_CLASSES.hidden);
        sidebar.classList.add(CSS_CLASSES.hidden);
        stopBtn.style.display = DISPLAY_STYLES.flex;
        startBtn.style.display = DISPLAY_STYLES.none;
    } else {
        hud.classList.add(CSS_CLASSES.hidden);
        sidebar.classList.remove(CSS_CLASSES.hidden);
        stopBtn.style.display = DISPLAY_STYLES.none;
        startBtn.style.display = '';
    }
}
