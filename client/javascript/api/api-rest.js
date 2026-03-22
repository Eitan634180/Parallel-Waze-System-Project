import { SERVER_URL } from '../core/config.js';

async function parseJsonResponse(res) {
    if (!res.ok) {
        const message = await res.text();
        throw new Error(message || `Request failed with status ${res.status}`);
    }
    return res.json();
}

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
    const res = await fetch(`${SERVER_URL}/session/${sessionId}`, { method: 'DELETE' });
    if (!res.ok && res.status !== 404) {
        const message = await res.text();
        throw new Error(message || `Failed to delete session ${sessionId}`);
    }
}

export async function spawnSimulationCars(routeIds, count, minStepIndex = 0) {
    const res = await fetch(`${SERVER_URL}/simulation`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ route_ids: routeIds, count, min_step_index: minStepIndex }),
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
    const res = await fetch(`${SERVER_URL}/simulation`, { method: 'DELETE' });
    if (!res.ok && res.status !== 404) {
        const message = await res.text();
        throw new Error(message || 'Failed to clear simulation cars');
    }
}