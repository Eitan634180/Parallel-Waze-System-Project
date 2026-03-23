import { formatDuration, getArrivalTime } from '../../shared/utils/formatters.js';

export function updateETA(seconds) {
    const durationStr = formatDuration(seconds);
    document.getElementById('eta-value').textContent = durationStr;
    document.getElementById('hud-eta').textContent = durationStr;
    document.getElementById('hud-arrival-time').textContent = getArrivalTime(seconds);
}

export function updateDistance(meters, elementId = 'distance-value') {
    const km = (meters / 1000).toFixed(1);
    document.getElementById(elementId).textContent = `${km} km`;
}

export function updateTrafficStatus(status = 'normal', detail = '') {
    const el = document.getElementById('traffic-status');
    if (!el) return;

    if (typeof status === 'boolean') status = status ? 'congested' : 'normal';

    if (status === 'heavy') {
        el.textContent = 'Heavy';
        el.className = 'stat-value heavy';
    } else if (status === 'congested') {
        el.textContent = 'Slower';
        el.className = 'stat-value congested';
    } else {
        el.textContent = 'Normal';
        el.className = 'stat-value';
    }
    el.title = detail || '';
}

export function toggleDrivingHUD(isDriving) {
    const hud = document.getElementById('driving-hud');
    const sidebar = document.getElementById('sidebar');
    const stopBtn = document.getElementById('stop-drive-btn');
    const startBtn = document.getElementById('start-drive-btn');

    if (isDriving) {
        hud.classList.remove('hidden');
        sidebar.classList.add('hidden');
        stopBtn.style.display = 'flex';
        startBtn.style.display = 'none';
    } else {
        hud.classList.add('hidden');
        sidebar.classList.remove('hidden');
        stopBtn.style.display = 'none';
        startBtn.style.display = '';
    }
}
