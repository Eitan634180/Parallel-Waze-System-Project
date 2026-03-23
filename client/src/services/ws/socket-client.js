import { SERVER_URL } from '../../app/app-config.js';

let socket = null;
let simulationSocket = null;

export function connectToSession(sessionId, callbacks) {
    if (socket) {
        socket.onclose = null;
        socket.close();
    }

    const wsUrl = SERVER_URL.replace(/^http/, 'ws') + `/session/${sessionId}/ws`;

    return new Promise((resolve, reject) => {
        let opened = false;
        let settled = false;
        socket = new WebSocket(wsUrl);

        socket.onopen = () => {
            opened = true;
            settled = true;
            console.log('Connected to navigation session');
            resolve();
        };

        socket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                if (message.type === 'eta_update' && callbacks.onEtaUpdate) callbacks.onEtaUpdate(message);
                else if (message.type === 'reroute' && callbacks.onReroute) callbacks.onReroute(message);
                else if (message.type === 'speed_update' && callbacks.onSpeedUpdate) callbacks.onSpeedUpdate(message);
                else if (message.type === 'debug_update' && callbacks.onDebugUpdate) callbacks.onDebugUpdate(message.debug);
            } catch (err) {
                console.error('Failed to parse websocket message:', err);
            }
        };

        socket.onerror = (error) => {
            console.error('WebSocket encountered an error:', error);
            if (!settled) {
                settled = true;
                reject(new Error('WebSocket connection failed'));
            }
        };

        socket.onclose = (event) => {
            socket = null;
            if (!settled) {
                settled = true;
                reject(new Error('WebSocket connection failed'));
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
            type: 'ping', lat, lon: lng, speed_kmh: speedKmh, step_index: stepIndex, edge_events: edgeEvents,
        }));
    }
}

export function connectToSimulation(callbacks) {
    if (simulationSocket) {
        simulationSocket.onclose = null;
        simulationSocket.close();
    }

    const wsUrl = SERVER_URL.replace(/^http/, 'ws') + '/simulation/ws';

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
                if (message.type === 'snapshot' && callbacks.onSnapshot) callbacks.onSnapshot(message.cars || []);
            } catch (err) {
                console.error('Failed to parse simulation websocket message:', err);
            }
        };

        simulationSocket.onerror = () => {
            if (!settled) {
                settled = true;
                reject(new Error('Simulation websocket connection failed'));
            }
        };
        simulationSocket.onclose = (event) => {
            simulationSocket = null;
            if (!settled) {
                settled = true;
                reject(new Error('Simulation websocket connection failed'));
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
