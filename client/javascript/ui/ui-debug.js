import { formatDuration } from '../utils/utils.js';

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
        el.innerHTML = `
            <strong>No edge selected</strong>
            <div>Click a highlighted route segment to inspect its road data.</div>
        `;
        return;
    }

    el.innerHTML = `
        <strong>Edge ${edge.edgeId ?? '--'} | Segment ${edge.index + 1}</strong>
        <div class="debug-status-grid">
            <div><strong>From</strong><br>${edge.fromNodeId}</div>
            <div><strong>To</strong><br>${edge.toNodeId}</div>
            <div><strong>Distance</strong><br>${(edge.distanceM || 0).toFixed(1)} m</div>
            <div><strong>Base time</strong><br>${(edge.durationSec || 0).toFixed(1)} s</div>
            <div><strong>Speed limit</strong><br>${edge.speedLimitKmh || '--'} km/h</div>
            <div><strong>Coords</strong><br>${edge.fromLat.toFixed(5)}, ${edge.fromLon.toFixed(5)}</div>
        </div>
    `;
}

export function renderDebugCarStatus(status) {
    const el = document.getElementById('debug-car-status');
    if (!el) return;

    el.innerHTML = `
        <strong>Simulation feed</strong>
        <div>Active test cars: <strong>${status.active}</strong></div>
    `;
}

export function renderMainCarDebug(status) {
    const el = document.getElementById('debug-main-status');
    if (!el) return;
    if (!status?.session_id) {
        el.innerHTML = `
            <strong>Main car session</strong>
            <div>Start driving to inspect live navigation state.</div>
        `;
        return;
    }

    const rerouteAt = status.last_reroute_at_unix_ms ? new Date(status.last_reroute_at_unix_ms).toLocaleTimeString() : '--';
    const rerouteReason = status.last_reroute_reason ? status.last_reroute_reason.replace('_', ' ') : 'none';
    const currentEdge = status.current_edge_id ?? '--';
    const trafficState = status.congestion_ahead ? `${status.congested_edges} congested edge${status.congested_edges === 1 ? '' : 's'} ahead` : 'clear ahead';

    el.innerHTML = `
        <strong>Main car session</strong>
        <div class="debug-status-grid">
            <div><strong>Session</strong><br>${status.session_id}</div>
            <div><strong>Step / Edge</strong><br>${status.step_index} / ${currentEdge}</div>
            <div><strong>Speed</strong><br>${Math.round(status.speed_kmh || 0)} km/h</div>
            <div><strong>ETA</strong><br>${formatDuration(status.eta_sec)}</div>
            <div><strong>Off-route</strong><br>${(status.off_route_distance_m || 0).toFixed(1)} / ${(status.off_route_threshold_m || 0).toFixed(0)} m</div>
            <div><strong>Strikes</strong><br>${status.off_route_violations || 0}</div>
            <div><strong>Traffic</strong><br>${trafficState}</div>
            <div><strong>Last reroute</strong><br>${rerouteReason} @ ${rerouteAt}</div>
        </div>
    `;
}
