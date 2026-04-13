import test, { afterEach } from 'node:test';
import assert from 'node:assert/strict';

import {
    connectToSession,
    connectToSimulation,
    disconnectSession,
    disconnectSimulation,
    sendLocationPing,
} from '../../src/services/ws/socket-client.js';

class MockWebSocket {
    static OPEN = 1;
    static instances = [];

    constructor(url) {
        this.url = url;
        this.readyState = 0;
        this.sent = [];
        this.closed = false;
        MockWebSocket.instances.push(this);
    }

    send(payload) {
        this.sent.push(JSON.parse(payload));
    }

    close() {
        this.closed = true;
        this.readyState = 3;
        if (this.onclose) {
            this.onclose({ code: 1000 });
        }
    }

    open() {
        this.readyState = MockWebSocket.OPEN;
        if (this.onopen) {
            this.onopen();
        }
    }

    message(payload) {
        if (this.onmessage) {
            this.onmessage({ data: JSON.stringify(payload) });
        }
    }

    fail(error = new Error('socket failed')) {
        if (this.onerror) {
            this.onerror(error);
        }
    }
}

const originalWebSocket = global.WebSocket;

afterEach(() => {
    disconnectSession();
    disconnectSimulation();
    global.WebSocket = originalWebSocket;
    MockWebSocket.instances.length = 0;
});

test('session websocket routes messages and sends pings', async () => {
    global.WebSocket = MockWebSocket;
    const events = [];

    const connectPromise = connectToSession('session-1', {
        onEtaUpdate: (msg) => events.push(['eta', msg.eta_sec]),
        onDebugUpdate: (msg) => events.push(['debug', msg.session_id]),
    });
    const socket = MockWebSocket.instances[0];
    socket.open();
    await connectPromise;

    socket.message({ type: 'eta_update', eta_sec: 42 });
    socket.message({ type: 'debug_update', debug: { session_id: 'session-1' } });
    sendLocationPing(1, 2, 30, 4, [{ edge_id: 9, observed_sec: 12 }]);

    assert.deepEqual(events, [['eta', 42], ['debug', 'session-1']]);
    assert.deepEqual(socket.sent[0], {
        type: 'ping',
        lat: 1,
        lon: 2,
        speed_kmh: 30,
        step_index: 4,
        edge_events: [{ edge_id: 9, observed_sec: 12 }],
    });
});

test('simulation websocket resolves and emits snapshots', async () => {
    global.WebSocket = MockWebSocket;
    const snapshots = [];

    const connectPromise = connectToSimulation({
        onSnapshot: (cars) => snapshots.push(cars),
    });
    const socket = MockWebSocket.instances[0];
    socket.open();
    await connectPromise;

    socket.message({ type: 'snapshot', cars: [{ id: 'sim-1' }] });
    assert.deepEqual(snapshots, [[{ id: 'sim-1' }]]);
});
