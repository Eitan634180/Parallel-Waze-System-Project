import { formatDuration } from '../../utils/formatters.js';
import { DOM_IDS, PANEL_NUMBERS, PANEL_TEXT } from '../ui-constants.js';

const EMPTY_VALUE = '--';
const EDGE_ID_FALLBACK = '--';
const REROUTE_REASON_FALLBACK = 'none';
const CLEAR_AHEAD_TEXT = 'clear ahead';

export function toggleDebugPanel(isOpen) {
    const panel = document.getElementById(DOM_IDS.debugPanel);
    if (!panel) return;
    panel.classList.toggle('hidden', !isOpen);
}

export function setRouteInspectorButtonState(isActive) {
    const btn = document.getElementById(DOM_IDS.toggleRouteInspectorButton);
    if (!btn) return;
    btn.classList.toggle('active', isActive);
    btn.textContent = isActive ? PANEL_TEXT.disableRouteInspector : PANEL_TEXT.inspectRouteEdges;
}

export function renderEdgeDebugInfo(edge) {
    const el = document.getElementById(DOM_IDS.debugEdgeInfo);
    if (!el) return;
    if (!edge) {
        el.innerHTML = `
            <strong>${PANEL_TEXT.noEdgeSelected}</strong>
            <div>${PANEL_TEXT.edgeInfoHint}</div>
        `;
        return;
    }

    el.innerHTML = `
        <strong>Edge ${edge.edgeId ?? EDGE_ID_FALLBACK} | Segment ${edge.index + 1}</strong>
        <div class="debug-status-grid">
            <div><strong>From</strong><br>${edge.fromNodeId}</div>
            <div><strong>To</strong><br>${edge.toNodeId}</div>
            <div><strong>Distance</strong><br>${(edge.distanceM || 0).toFixed(PANEL_NUMBERS.edgeInfoDistancePrecision)} m</div>
            <div><strong>Base time</strong><br>${(edge.durationSec || 0).toFixed(PANEL_NUMBERS.edgeInfoDurationPrecision)} s</div>
            <div><strong>Speed limit</strong><br>${edge.speedLimitKmh || EMPTY_VALUE} km/h</div>
            <div><strong>Coords</strong><br>${edge.fromLat.toFixed(PANEL_NUMBERS.edgeInfoCoordinatePrecision)}, ${edge.fromLon.toFixed(PANEL_NUMBERS.edgeInfoCoordinatePrecision)}</div>
        </div>
    `;
}

export function renderDebugCarStatus(status) {
    const el = document.getElementById(DOM_IDS.debugCarStatus);
    if (!el) return;

    el.innerHTML = `
        <strong>${PANEL_TEXT.simulationFeed}</strong>
        <div>Active test cars: <strong>${status.active}</strong></div>
    `;
}

export function renderMainCarDebug(status) {
    const el = document.getElementById(DOM_IDS.debugMainStatus);
    if (!el) return;
    if (!status?.session_id) {
        el.innerHTML = `
            <strong>Main car session</strong>
            <div>${PANEL_TEXT.mainCarHint}</div>
        `;
        return;
    }

    const rerouteAt = status.last_reroute_at_unix_ms ? new Date(status.last_reroute_at_unix_ms).toLocaleTimeString() : EMPTY_VALUE;
    const rerouteReason = status.last_reroute_reason ? status.last_reroute_reason.replace('_', ' ') : REROUTE_REASON_FALLBACK;
    const currentEdge = status.current_edge_id ?? EMPTY_VALUE;
    const trafficState = status.congestion_ahead ? `${status.congested_edges} congested edge${status.congested_edges === 1 ? '' : 's'} ahead` : CLEAR_AHEAD_TEXT;

    el.innerHTML = `
        <strong>Main car session</strong>
        <div class="debug-status-grid">
            <div><strong>Session</strong><br>${status.session_id}</div>
            <div><strong>Step / Edge</strong><br>${status.step_index} / ${currentEdge}</div>
            <div><strong>Speed</strong><br>${Math.round(status.speed_kmh || 0).toFixed(PANEL_NUMBERS.speedDisplayPrecision)} km/h</div>
            <div><strong>ETA</strong><br>${formatDuration(status.eta_sec)}</div>
            <div><strong>Off-route</strong><br>${(status.off_route_distance_m || 0).toFixed(PANEL_NUMBERS.offRouteDistancePrecision)} / ${(status.off_route_threshold_m || 0).toFixed(PANEL_NUMBERS.offRouteThresholdPrecision)} m</div>
            <div><strong>Strikes</strong><br>${status.off_route_violations || 0}</div>
            <div><strong>Traffic</strong><br>${trafficState}</div>
            <div><strong>Last reroute</strong><br>${rerouteReason} @ ${rerouteAt}</div>
        </div>
    `;
}
