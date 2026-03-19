import { formatDuration, getArrivalTime } from './utils.js';
import { searchLocationByName } from './api.js';

export function setupSearchInput(inputId, suggestionsId, onSelect) {
    const input = document.getElementById(inputId);
    const dropdown = document.getElementById(suggestionsId);
    let searchTimeout = null;

    input.addEventListener('input', () => {

        // Search after 400 idle milliseconds
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(async () => {
            const results = await searchLocationByName(input.value);
            dropdown.innerHTML = '';
            if (results.length > 0) {

                // Create dropdown menu
                dropdown.classList.add('active');
                results.forEach((r, idx) => {

                    const div = document.createElement('div');
                    div.className = 'suggestion-item';
                    const span = document.createElement('span');
                    span.textContent = r.displayName;
                    div.appendChild(span);

                    div.style.animationDelay = `${idx * 0.05}s`;
                    div.addEventListener('click', () => {
                        input.value = r.displayName;
                        dropdown.classList.remove('active');
                        onSelect({ lat: r.lat, lng: r.lng, name: r.displayName });
                    });
                    
                    dropdown.appendChild(div);
                });

            } else {
                dropdown.classList.remove('active');
            }
        }, 400);
    });

    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!e.target.closest(`#${inputId}`) && !e.target.closest(`#${suggestionsId}`)) {
            dropdown.classList.remove('active');
        }
    });
}

export function renderRouteOptions(routes, onSelectRoute) {
    const container = document.getElementById('route-options-container');
    container.innerHTML = '';

    routes.forEach((route, idx) => {

        // Create list element
        const isFastest = idx === 0;
        const div = document.createElement('div');
        div.className = `route-option-card ${idx === 0 ? 'active' : ''}`;
        div.id = `route-option-${idx}`;

        // Set route info
        const distKm = (route.distance / 1000).toFixed(1);
        const duration = formatDuration(route.dynamicETA);
        const arrivalTime = getArrivalTime(route.dynamicETA);

        div.innerHTML = `
            <div class="route-option-main">
                ${duration}
                ${isFastest ? '<span class="route-option-tag">Fastest</span>' : ''}
            </div>

            <div class="route-option-sub">
                <span>${distKm} km</span>
                <span class="route-sep">|</span>
                <span>Arrives ${arrivalTime}</span>
            </div>
        `;

        // Route selecting
        div.addEventListener('click', () => onSelectRoute(idx));
        container.appendChild(div);
    });
}

export function updateETA(seconds) {

    // Display new ETA
    const durationStr = formatDuration(seconds);
    document.getElementById('eta-value').textContent = durationStr;
    document.getElementById('hud-eta').textContent = durationStr;

    // Arrival time
    document.getElementById('hud-arrival-time').textContent = getArrivalTime(seconds);
}

export function updateDistance(meters, elementId = 'distance-value') {
    const km = (meters / 1000).toFixed(1);
    document.getElementById(elementId).textContent = `${km} km`;
}

export function updateTrafficStatus(status = 'normal', detail = '') {
    const el = document.getElementById('traffic-status');
    if (!el) return;

    if (typeof status === 'boolean') {
        status = status ? 'congested' : 'normal';
    }

    if (status === 'heavy') {
        el.textContent = 'Heavy';
        el.className = 'stat-value heavy';
    } else if (status === 'congested') {
        el.textContent = 'Slowdowns';
        el.className = 'stat-value congested';
    } else {
        el.textContent = 'Normal';
        el.className = 'stat-value';
    }
    el.title = detail || '';
}

export function showAlert(title, message) {
    const toast = document.getElementById('alert-toast');
    document.getElementById('alert-title').textContent = title;
    document.getElementById('alert-message').textContent = message;
    toast.classList.remove('hidden');
    toast.style.animation = 'toastIn 0.4s ease forwards';

    setTimeout(() => {
        toast.style.animation = 'toastOut 0.4s ease forwards';
        setTimeout(() => toast.classList.add('hidden'), 400);
    }, 5000);
}

export function onArrival(distance, eta) {
    const overlay = document.createElement('div');
    overlay.id = 'arrival-overlay';
    overlay.innerHTML = `
    <div class="arrival-card">
            <div class="arrival-icon">🚩</div>
            <h2>Arrived</h2>
            <p>You have reached your destination</p>
            <div class="arrival-stats">
                <div class="a-stat">
                    <span class="a-label">Distance</span>
                    <span class="a-val">${(distance / 1000).toFixed(1)} km</span>
                </div>
                <div class="a-stat">
                    <span class="a-label">Time</span>
                    <span class="a-val">${formatDuration(eta)}</span>
                </div>
            </div>
            <button onclick="document.getElementById('arrival-overlay').remove()">Close</button>
        </div>
    `;
    document.body.appendChild(overlay);
}

export function toggleLoadingState(isLoading) {
    document.getElementById('navigate-btn').disabled = isLoading;
    if (isLoading) document.body.classList.add('loading');
    else document.body.classList.remove('loading');
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

export function toggleDebugPanel(isOpen) {
    const panel = document.getElementById('debug-panel');
    if (!panel) return;
    panel.classList.toggle('hidden', !isOpen);
}

export function setRouteInspectorButtonState(isActive) {
    const btn = document.getElementById('toggle-route-inspector-btn');
    if (!btn) return;

    btn.classList.toggle('active', isActive);
    btn.textContent = isActive ? 'Disable Route Inspector' : 'Inspect Route Edges';
}

export function renderEdgeDebugInfo(edge) {
    const el = document.getElementById('debug-edge-info');
    if (!el) return;
    if (!edge) {
        el.textContent = 'Click an edge to see its details.';
        return;
    }

    el.innerHTML = `
        <strong>Edge ${edge.edgeId ?? '--'} | Segment ${edge.index + 1}</strong>
        <div>From node: ${edge.fromNodeId}</div>
        <div>To node: ${edge.toNodeId}</div>
        <div>Distance: ${(edge.distanceM || 0).toFixed(1)} m</div>
        <div>Base time: ${(edge.durationSec || 0).toFixed(1)} s</div>
        <div>Speed limit: ${edge.speedLimitKmh || '--'} km/h</div>
        <div>From: ${edge.fromLat.toFixed(5)}, ${edge.fromLon.toFixed(5)}</div>
        <div>To: ${edge.toLat.toFixed(5)}, ${edge.toLon.toFixed(5)}</div>
    `;
}

export function renderDebugCarStatus(status) {
    const el = document.getElementById('debug-car-status');
    if (!el) return;
    el.innerHTML = `
        Active debug cars: ${status.active}<br>
        Created: ${status.created}<br>
        Completed: ${status.completed}
    `;
}

export function renderMainCarDebug(status) {
    const el = document.getElementById('debug-main-status');
    if (!el) return;
    if (!status?.session_id) {
        el.textContent = 'Start driving to inspect the main car session.';
        return;
    }

    const rerouteAt = status.last_reroute_at_unix_ms
        ? new Date(status.last_reroute_at_unix_ms).toLocaleTimeString()
        : '--';
    const rerouteReason = status.last_reroute_reason
        ? status.last_reroute_reason.replace('_', ' ')
        : 'none';
    const currentEdge = status.current_edge_id ?? '--';
    const trafficState = status.congestion_ahead
        ? `${status.congested_edges} congested edge${status.congested_edges === 1 ? '' : 's'} ahead`
        : 'clear ahead';

    el.innerHTML = `
        <strong>Main Car</strong>
        <div>Session: ${status.session_id}</div>
        <div>Step: ${status.step_index} | Edge: ${currentEdge}</div>
        <div>Speed: ${Math.round(status.speed_kmh || 0)} km/h | ETA: ${formatDuration(status.eta_sec)}</div>
        <div>Off-route: ${(status.off_route_distance_m || 0).toFixed(1)} / ${(status.off_route_threshold_m || 0).toFixed(0)} m</div>
        <div>Off-route strikes: ${status.off_route_violations || 0}</div>
        <div>Traffic: ${trafficState}</div>
        <div>Last reroute: ${rerouteReason} @ ${rerouteAt}</div>
    `;
}
