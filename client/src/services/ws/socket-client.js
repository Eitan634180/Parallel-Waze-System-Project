import { SERVER_URL } from '../../app/app-config.js';
import { API_PATHS, WS_MESSAGE_TYPES, WS_PROTOCOL } from '../service-config.js';

const SOCKET_ERRORS = {
    sessionConnectionFailed: 'WebSocket connection failed',
    simulationConnectionFailed: 'Simulation websocket connection failed',
};
const SOCKET_LOG_MESSAGES = {
    sessionMessageParseFailed: 'Session websocket message parse failed',
    sessionSocketError: 'Session websocket error',
    simulationMessageParseFailed: 'Simulation websocket message parse failed',
};

function toWsUrl(path) {
    return SERVER_URL.replace(WS_PROTOCOL.httpSchemePattern, WS_PROTOCOL.wsScheme) + path;
}

let socket = null;
let simulationSocket = null;

export function connectToSession(sessionId, callbacks) {
    if (socket) {
        socket.onclose = null;
        socket.close();
    }

    const wsUrl = toWsUrl(`${API_PATHS.session}/${sessionId}/ws`);

    return new Promise((resolve, reject) => {
        let opened = false;
        let settled = false;
        socket = new WebSocket(wsUrl);

        socket.onopen = () => {
            opened = true;
            settled = true;
            resolve();
        };

        socket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.type === WS_MESSAGE_TYPES.etaUpdate && callbacks.onEtaUpdate) callbacks.onEtaUpdate(message);
                else if (message.type === WS_MESSAGE_TYPES.reroute && callbacks.onReroute) callbacks.onReroute(message);
                else if (message.type === WS_MESSAGE_TYPES.speedUpdate && callbacks.onSpeedUpdate) callbacks.onSpeedUpdate(message);
                else if (message.type === WS_MESSAGE_TYPES.debugUpdate && callbacks.onDebugUpdate) callbacks.onDebugUpdate(message.debug);
            } catch (err) {
                console.error(SOCKET_LOG_MESSAGES.sessionMessageParseFailed, err);
            }
        };

        socket.onerror = (error) => {
            console.error(SOCKET_LOG_MESSAGES.sessionSocketError, error);
            if (!settled) {
                settled = true;
                reject(new Error(SOCKET_ERRORS.sessionConnectionFailed));
            }
        };

        socket.onclose = (event) => {
            socket = null;
            if (!settled) {
                settled = true;
                reject(new Error(SOCKET_ERRORS.sessionConnectionFailed));
                return;
            }
            if (opened && callbacks.onClose) {
                callbacks.onClose(event);
            }
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
            type: WS_MESSAGE_TYPES.ping, lat, lon: lng, speed_kmh: speedKmh, step_index: stepIndex, edge_events: edgeEvents,
        }));
    }
}

export function connectToSimulation(callbacks) {
    if (simulationSocket) {
        simulationSocket.onclose = null;
        simulationSocket.close();
    }

    const wsUrl = toWsUrl(API_PATHS.simulationWS);

    return new Promise((resolve, reject) => {
        let settled = false;
        simulationSocket = new WebSocket(wsUrl);
        simulationSocket.onopen = () => {
            settled = true;
            resolve();
        };

        simulationSocket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.type === WS_MESSAGE_TYPES.snapshot && callbacks.onSnapshot) callbacks.onSnapshot(message.cars || []);
            } catch (err) {
                console.error(SOCKET_LOG_MESSAGES.simulationMessageParseFailed, err);
            }
        };

        simulationSocket.onerror = () => {
            if (!settled) {
                settled = true;
                reject(new Error(SOCKET_ERRORS.simulationConnectionFailed));
            }
        };
        simulationSocket.onclose = (event) => {
            simulationSocket = null;
            if (!settled) {
                settled = true;
                reject(new Error(SOCKET_ERRORS.simulationConnectionFailed));
                return;
            }
            if (callbacks.onClose) {
                callbacks.onClose(event);
            }
        };
    });
}

export function disconnectSimulation() {
    if (!simulationSocket) return;
    simulationSocket.onclose = null;
    simulationSocket.close();
    simulationSocket = null;
}
