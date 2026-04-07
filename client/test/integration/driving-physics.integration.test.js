import test from 'node:test';
import assert from 'node:assert/strict';

import { state, resetDrivingSession } from '../../src/app/app-state.js';
import { calculateNewPosition, limitMovementByTraffic } from '../../src/features/driving/driving-physics.js';
import { processRawRoute } from '../../src/features/routing/route-utils.js';
import { sampleRoutePayload } from '../fixtures/routes.js';

function loadDrivingFixture() {
    state.routing.allRoutes = [];
    state.routing.currentIndex = 0;
    state.routing.activeObj = processRawRoute(sampleRoutePayload);
    state.routing.activeLegs = state.routing.activeObj.legs;
    resetDrivingSession();
    state.routing.activeObj = processRawRoute(sampleRoutePayload);
    state.routing.activeLegs = state.routing.activeObj.legs;
    state.drive.carPos = [32.0000, 34.0000];
    state.drive.distanceLeft = state.routing.activeObj.distance;
    state.sim.pendingEdgeEvents = [];
    state.sim.currentEdgeTimeMs = 1000;
}

test('calculateNewPosition advances across edges and records completed edge events', () => {
    loadDrivingFixture();
    const nextPos = calculateNewPosition(120, 1000);

    assert.equal(state.drive.currentRoadIndex, 1);
    assert.equal(state.sim.pendingEdgeEvents.length, 1);
    assert.equal(state.sim.pendingEdgeEvents[0].edge_id, 11);
    assert.ok(Array.isArray(nextPos));
    assert.equal(nextPos.length, 2);
});

test('limitMovementByTraffic reduces movement when a simulated car is ahead on the route', () => {
    loadDrivingFixture();
    state.sim.cars = [{ id: 'sim-1', lat: 32.0000, lon: 34.0002 }];
    const limited = limitMovementByTraffic(80, 36);

    assert.ok(limited >= 0);
    assert.ok(limited < 80);
});
