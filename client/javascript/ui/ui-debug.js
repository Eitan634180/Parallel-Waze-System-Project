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
    
    el.innerHTML = `Active test cars: <strong>${status.active}</strong>`;
}

export function renderMainCarDebug(status) {
    const el = document.getElementById('debug-main-status');
    if (!el) return;
    if (!status?.session_id) {
        el.textContent = 'Start driving to inspect the main car session.';
        return;
    }

    const rerouteAt = status.last_reroute_at_unix_ms ? new Date(status.last_reroute_at_unix_ms).toLocaleTimeString() : '--';
    const rerouteReason = status.last_reroute_reason ? status.last_reroute_reason.replace('_', ' ') : 'none';
    const currentEdge = status.current_edge_id ?? '--';
    const trafficState = status.congestion_ahead ? `${status.congested_edges} congested edge${status.congested_edges === 1 ? '' : 's'} ahead` : 'clear ahead';

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