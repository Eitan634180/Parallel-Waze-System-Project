import { SERVER_URL } from './config.js';

// ==========================================
// REST API
// ==========================================

export async function searchLocationByName(query) {
    if (!query) return [];

    const coordinateMatch = query.match(/^\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*$/);
    if (coordinateMatch) {
        const lat = Number(coordinateMatch[1]);
        const lng = Number(coordinateMatch[2]);
        if (Math.abs(lat) <= 90 && Math.abs(lng) <= 180) {
            return [{ lat, lng, displayName: `${lat.toFixed(5)}, ${lng.toFixed(5)}` }];
        }
    }

    try {
        const res = await fetch(`${SERVER_URL}/search?q=${encodeURIComponent(query)}`);
        if (!res.ok) throw new Error('Search request failed');
        return await res.json();
    } catch (err) {
        console.error('Search error:', err);
        return [];
    }
}

async function parseJsonResponse(res) {
    if (!res.ok) {
        const message = await res.text();
        throw new Error(message || `Request failed with status ${res.status}`);
    }

    return res.json();
}

export async function fetchRoute(srcLat, srcLng, dstLat, dstLng, alternatives = 2) {
    const res = await fetch(`${SERVER_URL}/route`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            src_lat: srcLat,
            src_lon: srcLng,
            dst_lat: dstLat,
            dst_lon: dstLng,
            alternatives,
        }),
    });

    const data = await parseJsonResponse(res);
    return data.routes;
}

// ==========================================
// WEBSOCKET API
// ==========================================

let socket = null;
let simulationSocket = null;

export async function createSession(routeId) {
    const res = await fetch(`${SERVER_URL}/session`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ route_id: routeId }),
    });

    const data = await parseJsonResponse(res);
    return data.session_id;
}

export async function deleteSession(sessionId) {
    if (!sessionId) return;

    const res = await fetch(`${SERVER_URL}/session/${sessionId}`, {
        method: 'DELETE',
    });

    if (!res.ok && res.status !== 404) {
        const message = await res.text();
        throw new Error(message || `Failed to delete session ${sessionId}`);
    }
}

export function connectToSession(sessionId, callbacks) {
    if (socket) {
        socket.onclose = null;
        socket.close();
    }

    const wsUrl = SERVER_URL.replace(/^http/, 'ws') + `/session/${sessionId}/ws`;

    return new Promise((resolve, reject) => {
        socket = new WebSocket(wsUrl);

        socket.onopen = () => {
            console.log('Connected to navigation session');
            resolve();
        };

        socket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);

                if (message.type === 'eta_update' && callbacks.onEtaUpdate) {
                    callbacks.onEtaUpdate(message);
                } else if (message.type === 'reroute' && callbacks.onReroute) {
                    callbacks.onReroute(message);
                } else if (message.type === 'speed_update' && callbacks.onSpeedUpdate) {
                    callbacks.onSpeedUpdate(message);
                } else if (message.type === 'debug_update' && callbacks.onDebugUpdate) {
                    callbacks.onDebugUpdate(message.debug);
                }
            } catch (err) {
                console.error('Failed to parse websocket message:', err);
            }
        };

        socket.onerror = (error) => {
            console.error('WebSocket encountered an error:', error);
            reject(new Error('WebSocket connection failed'));
        };

        socket.onclose = () => {
            socket = null;
        };
    });
}

export function disconnectSession() {
    if (!socket) return;
    socket.onclose = null;
    socket.close();
    socket = null;
}

export function sendLocationPing(lat, lng, speedKmh, stepIndex, edgeEvents = []) {
    if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({
            type: 'ping',
            lat,
            lon: lng,
            speed_kmh: speedKmh,
            step_index: stepIndex,
            edge_events: edgeEvents,
        }));
    }
}

export async function spawnSimulationCars(routeIds, count, minStepIndex = 0) {
    const res = await fetch(`${SERVER_URL}/simulation`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            route_ids: routeIds,
            count,
            min_step_index: minStepIndex 
        }),
    });

    return parseJsonResponse(res);
}

export async function spawnRandomSimulationCars(count) {
    const res = await fetch(`${SERVER_URL}/simulation/random`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ count }),
    });

    return parseJsonResponse(res);
}

export async function clearSimulationCars() {
    const res = await fetch(`${SERVER_URL}/simulation`, {
        method: 'DELETE',
    });

    if (!res.ok && res.status !== 404) {
        const message = await res.text();
        throw new Error(message || 'Failed to clear simulation cars');
    }
}

export function connectToSimulation(callbacks) {
    if (simulationSocket) {
        simulationSocket.onclose = null;
        simulationSocket.close();
    }

    const wsUrl = SERVER_URL.replace(/^http/, 'ws') + '/simulation/ws';

    return new Promise((resolve, reject) => {
        simulationSocket = new WebSocket(wsUrl);

        simulationSocket.onopen = () => {
            resolve();
        };

        simulationSocket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.type === 'snapshot' && callbacks.onSnapshot) {
                    callbacks.onSnapshot(message.cars || []);
                }
            } catch (err) {
                console.error('Failed to parse simulation websocket message:', err);
            }
        };

        simulationSocket.onerror = () => {
            reject(new Error('Simulation websocket connection failed'));
        };

        simulationSocket.onclose = () => {
            simulationSocket = null;
        };
    });
}

export function disconnectSimulation() {
    if (!simulationSocket) return;
    simulationSocket.onclose = null;
    simulationSocket.close();
    simulationSocket = null;
}
