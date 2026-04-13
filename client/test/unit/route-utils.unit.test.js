import test from 'node:test';
import assert from 'node:assert/strict';

import { sampleRoutePayload } from '../fixtures/routes.js';
import { processRawRoute, projectPositionOntoRoute } from '../../src/features/routing/route-utils.js';

test('processRawRoute converts cumulative steps into route legs', () => {
    const route = processRawRoute(sampleRoutePayload);

    assert.equal(route.id, 'route-1');
    assert.equal(route.legs.length, 2);
    assert.deepEqual(route.pathCoords, [
        [34.0000, 32.0000],
        [34.0010, 32.0000],
        [34.0010, 32.0010],
    ]);
    assert.equal(route.legs[0].base_length, 100);
    assert.equal(route.legs[0].duration_sec, 10);
    assert.equal(route.legs[0].edge_id, 11);
    assert.equal(route.distance, 220);
    assert.equal(route.dynamicETA, 25);
});

test('projectPositionOntoRoute finds the nearest segment and remaining distance', () => {
    const route = processRawRoute(sampleRoutePayload);
    const projected = projectPositionOntoRoute(route, 32.0000, 34.0006);

    assert.ok(projected);
    assert.equal(projected.roadIndex, 0);
    assert.ok(projected.stepProgress > 0);
    assert.ok(projected.distanceLeft < route.distance);
    assert.ok(projected.distanceLeftOnStep > 0);
    assert.ok(projected.offsetM < 5);
});
