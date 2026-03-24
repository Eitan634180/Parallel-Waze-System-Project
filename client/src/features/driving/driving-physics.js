import { state } from '../../app/app-state.js';
import { projectPositionOntoRoute } from '../routing/route-utils.js';

const FOLLOWING_TIME_SEC = 1.8;
const MIN_GAP_M = 7;
const MAX_TRACKED_GAP_M = 80;
const ROUTE_CAPTURE_M = 18;

export function calculateNewPosition(metersToMove, elapsedMs) {
    let remainingMeters = metersToMove;
    let remainingElapsedMs = elapsedMs;
    let pos = state.drive.carPos;

    // Move through the route, segment by segment
    while (remainingMeters > 0 && state.drive.currentRoadIndex < state.routing.activeLegs.length) {
        const step = state.routing.activeLegs[state.drive.currentRoadIndex];
        const stepDist = step.base_length || 0.1; 
        const distanceLeftOnStep = stepDist - state.drive.stepProgress;

        const startPos = [step.from_node[1], step.from_node[0]];
        const endPos = [step.to_node[1], step.to_node[0]];

        if (distanceLeftOnStep <= remainingMeters) {
            // Segment finished: record travel time and jump to the next road segment
            const spilloverMs = remainingMeters > 0 ? remainingElapsedMs * ((remainingMeters - distanceLeftOnStep) / remainingMeters) : 0;
            const observedMs = Math.max(0, state.sim.currentEdgeTimeMs - spilloverMs);
            remainingMeters -= distanceLeftOnStep;
            remainingElapsedMs = spilloverMs;
            
            if (step.edge_id !== null && step.edge_id !== undefined && observedMs > 0) {
                state.sim.pendingEdgeEvents.push({ edge_id: step.edge_id, observed_sec: observedMs / 1000 });
            }
            state.sim.currentEdgeTimeMs = spilloverMs;
            state.drive.currentRoadIndex++;
            state.drive.stepProgress = 0;
            pos = endPos; 
        } else {
            // Segment ongoing: interpolate current position between start and end
            state.drive.stepProgress += remainingMeters;
            const fraction = state.drive.stepProgress / stepDist;
            pos = [startPos[0] + (endPos[0] - startPos[0]) * fraction, startPos[1] + (endPos[1] - startPos[1]) * fraction];
            remainingMeters = 0;
            remainingElapsedMs = 0;
        }
    }

    // Apply off route offset
    if (state.drive.isDrifting) {
        state.drive.offRouteOffset[0] += 0.0000006 * elapsedMs;
        state.drive.offRouteOffset[1] += 0.0000006 * elapsedMs;
    }

    return [pos[0] + state.drive.offRouteOffset[0], pos[1] + state.drive.offRouteOffset[1]];
}

export function limitMovementByTraffic(metersToMove, speedKmh) {
    if (!state.routing.activeObj || !state.drive.carPos) return metersToMove;

    const myProgressM = Math.max(0, (state.routing.activeObj.distance || 0) - (state.drive.distanceLeft || 0));
    const safetyGapM = Math.max(MIN_GAP_M, (speedKmh / 3.6) * FOLLOWING_TIME_SEC);
    let allowedMoveM = metersToMove;

    for (const car of state.sim.cars) {
        const projected = projectPositionOntoRoute(state.routing.activeObj, car.lat, car.lon);
        if (!projected || projected.offsetM > ROUTE_CAPTURE_M) continue;

        const otherProgressM = Math.max(0, (state.routing.activeObj.distance || 0) - projected.distanceLeft);
        const gapM = otherProgressM - myProgressM;

        // If a car is ahead, reduce allowed movement to maintain the safety gap
        if (gapM > 0 && gapM <= MAX_TRACKED_GAP_M) {
            allowedMoveM = Math.min(allowedMoveM, Math.max(0, gapM - safetyGapM));
        }
    }
    return Math.max(0, allowedMoveM);
}

export function calculateBearing(currentPos) {
    if (state.drive.currentRoadIndex >= state.routing.activeLegs.length) return 0;
    const step = state.routing.activeLegs[state.drive.currentRoadIndex];
    const nextPos = [step.to_node[1], step.to_node[0]];
    return Math.atan2(nextPos[1] - currentPos[1], nextPos[0] - currentPos[0]) * 180 / Math.PI;
}
