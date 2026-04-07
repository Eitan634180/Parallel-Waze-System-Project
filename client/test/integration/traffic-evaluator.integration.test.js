import test, { beforeEach } from 'node:test';
import assert from 'node:assert/strict';

import { state, resetDrivingSession } from '../../src/app/app-state.js';
import { processRawRoute } from '../../src/features/routing/route-utils.js';
import { computeTrafficStatus } from '../../src/features/driving/traffic-evaluator.js';
import { sampleRoutePayload } from '../fixtures/routes.js';

beforeEach(() => {
    state.routing.activeObj = processRawRoute(sampleRoutePayload);
    state.routing.activeLegs = state.routing.activeObj.legs;
    resetDrivingSession();
});

test('computeTrafficStatus prefers server debug state when available', () => {
    state.debug.serverData = {
        congestion_ahead: true,
        congested_edges: 2,
    };

    const status = computeTrafficStatus();
    assert.equal(status.level, 'heavy');
});

test('computeTrafficStatus derives slowdown from recommended edge speeds', () => {
    state.debug.serverData = null;
    state.drive.isActive = true;
    state.sim.recommendedSpeeds.set(11, 25);

    const status = computeTrafficStatus();
    assert.equal(status.level, 'congested');
});
