export const HTTP_METHODS = {
    delete: 'DELETE',
    get: 'GET',
    post: 'POST',
};

export const HTTP_HEADERS = {
    contentType: 'Content-Type',
};

export const HTTP_CONTENT_TYPES = {
    json: 'application/json',
};

export const HTTP_STATUS = {
    notFound: 404,
};

export const API_PATHS = {
    route: '/route',
    search: '/search',
    session: '/session',
    simulation: '/simulation',
    simulationRandom: '/simulation/random',
    simulationWS: '/simulation/ws',
    systemInfo: '/system/info',
};

export const WS_PROTOCOL = {
    httpSchemePattern: /^http/,
    wsScheme: 'ws',
};

export const WS_MESSAGE_TYPES = {
    debugUpdate: 'debug_update',
    etaUpdate: 'eta_update',
    ping: 'ping',
    reroute: 'reroute',
    snapshot: 'snapshot',
    speedUpdate: 'speed_update',
};

export const REQUEST_DEFAULTS = {
    coordinatePrecision: 5,
    defaultRouteAlternatives: 2,
};

export const SEARCH_LIMITS = {
    latitude: 90,
    longitude: 180,
};

export const SEARCH_PATTERNS = {
    latLng: /^\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*$/,
};

