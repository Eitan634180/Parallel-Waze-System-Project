import { SERVER_URL } from '../../app/app-config.js';
import {
    API_PATHS,
    HTTP_CONTENT_TYPES,
    HTTP_HEADERS,
    HTTP_METHODS,
    HTTP_STATUS,
    REQUEST_DEFAULTS,
    SEARCH_LIMITS,
    SEARCH_PATTERNS,
} from '../service-config.js';

const REQUEST_ERRORS = {
    clearSimulationCarsFailed: 'Failed to clear simulation cars',
    searchFailed: 'Search request failed',
};
const REQUEST_LOG_MESSAGES = {
    searchFailed: 'Search request failed',
};

async function parseJsonResponse(res) {
    if (!res.ok) {
        const message = await res.text();
        throw new Error(message || `Request failed with status ${res.status}`);
    }
    return res.json();
}

export async function searchLocationByName(query) {
    if (!query) return [];

    const coordinateMatch = query.match(SEARCH_PATTERNS.latLng);
    if (coordinateMatch) {
        const lat = Number(coordinateMatch[1]);
        const lng = Number(coordinateMatch[2]);
        if (Math.abs(lat) <= SEARCH_LIMITS.latitude && Math.abs(lng) <= SEARCH_LIMITS.longitude) {
            return [{ lat, lng, displayName: `${lat.toFixed(REQUEST_DEFAULTS.coordinatePrecision)}, ${lng.toFixed(REQUEST_DEFAULTS.coordinatePrecision)}` }];
        }
    }

    try {
        const res = await fetch(`${SERVER_URL}${API_PATHS.search}?q=${encodeURIComponent(query)}`);
        if (!res.ok) throw new Error(REQUEST_ERRORS.searchFailed);
        return await res.json();
    } catch (err) {
        console.error(REQUEST_LOG_MESSAGES.searchFailed, err);
        return [];
    }
}

export async function fetchRoute(srcLat, srcLng, dstLat, dstLng, alternatives = REQUEST_DEFAULTS.defaultRouteAlternatives) {
    const res = await fetch(`${SERVER_URL}${API_PATHS.route}`, {
        method: HTTP_METHODS.post,
        headers: { [HTTP_HEADERS.contentType]: HTTP_CONTENT_TYPES.json },
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
    const res = await fetch(`${SERVER_URL}${API_PATHS.session}`, {
        method: HTTP_METHODS.post,
        headers: { [HTTP_HEADERS.contentType]: HTTP_CONTENT_TYPES.json },
        body: JSON.stringify({ route_id: routeId }),
    });
    const data = await parseJsonResponse(res);
    return data.session_id;
}

export async function deleteSession(sessionId) {
    if (!sessionId) return;
    const res = await fetch(`${SERVER_URL}${API_PATHS.session}/${sessionId}`, { method: HTTP_METHODS.delete });
    if (!res.ok && res.status !== HTTP_STATUS.notFound) {
        const message = await res.text();
        throw new Error(message || `Failed to delete session ${sessionId}`);
    }
}

export async function spawnSimulationCars(routeIds, count, minStepIndex = 0) {
    const res = await fetch(`${SERVER_URL}${API_PATHS.simulation}`, {
        method: HTTP_METHODS.post,
        headers: { [HTTP_HEADERS.contentType]: HTTP_CONTENT_TYPES.json },
        body: JSON.stringify({ route_ids: routeIds, count, min_step_index: minStepIndex }),
    });
    return parseJsonResponse(res);
}

export async function spawnRandomSimulationCars(count) {
    const res = await fetch(`${SERVER_URL}${API_PATHS.simulationRandom}`, {
        method: HTTP_METHODS.post,
        headers: { [HTTP_HEADERS.contentType]: HTTP_CONTENT_TYPES.json },
        body: JSON.stringify({ count }),
    });
    return parseJsonResponse(res);
}

export async function clearSimulationCars() {
    const res = await fetch(`${SERVER_URL}${API_PATHS.simulation}`, { method: HTTP_METHODS.delete });
    if (!res.ok && res.status !== HTTP_STATUS.notFound) {
        const message = await res.text();
        throw new Error(message || REQUEST_ERRORS.clearSimulationCarsFailed);
    }
}
