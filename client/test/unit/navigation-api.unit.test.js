import test, { afterEach } from 'node:test';
import assert from 'node:assert/strict';

import {
    createSession,
    deleteSession,
    fetchRoute,
    searchLocationByName,
} from '../../src/services/rest/navigation-api.js';

let originalFetch = global.fetch;

afterEach(() => {
    global.fetch = originalFetch;
});

test('searchLocationByName handles coordinate input locally', async () => {
    let fetchCalls = 0;
    global.fetch = async () => {
        fetchCalls++;
        throw new Error('should not be called');
    };

    const result = await searchLocationByName('32.0853, 34.7818');
    assert.equal(fetchCalls, 0);
    assert.deepEqual(result, [{ lat: 32.0853, lng: 34.7818, displayName: '32.08530, 34.78180' }]);
});

test('fetchRoute posts coordinates and returns route payloads', async () => {
    let request;
    global.fetch = async (url, options) => {
        request = { url, options };
        return {
            ok: true,
            async json() {
                return { routes: [{ id: 'route-1' }] };
            },
        };
    };

    const routes = await fetchRoute(1, 2, 3, 4, 2);
    assert.deepEqual(routes, [{ id: 'route-1' }]);
    assert.match(request.url, /\/route$/);
    assert.equal(request.options.method, 'POST');
    assert.deepEqual(JSON.parse(request.options.body), {
        src_lat: 1,
        src_lon: 2,
        dst_lat: 3,
        dst_lon: 4,
        alternatives: 2,
    });
});

test('createSession returns the created session id', async () => {
    global.fetch = async () => ({
        ok: true,
        async json() {
            return { session_id: 'session-123' };
        },
    });

    const sessionId = await createSession('route-1');
    assert.equal(sessionId, 'session-123');
});

test('deleteSession tolerates 404 responses', async () => {
    global.fetch = async () => ({
        ok: false,
        status: 404,
        async text() {
            return 'missing';
        },
    });

    await assert.doesNotReject(() => deleteSession('session-123'));
});
