import { state } from '../../app/app-state.js';
import { projectPositionOntoRoute } from '../routing/route-utils.js';

const FOLLOWING_TIME_SEC = 1.8;
const MIN_GAP_M = 7;
const MAX_TRACKED_GAP_M = 80;
const ROUTE_CAPTURE_M = 18;
const MIN_STEP_DISTANCE_M = 0.1;
const MS_PER_SECOND = 1000;
const DEGREES_PER_RADIAN = 180 / Math.PI;
const HALF_TURN_DEG = 180;
const FULL_TURN_DEG = 360;
const SHARP_TURN_THRESHOLD_DEG = 25;
const TURN_SPEED_CAP_KMH = 20;
const OFF_ROUTE_DRIFT_PER_MS = 0.0000006;

export function calculateNewPosition(metersToMove, elapsedMs) {
    let remainingMeters = metersToMove;
    let remainingElapsedMs = elapsedMs;
    let pos = state.drive.carPos;

    while (remainingMeters > 0 && state.drive.currentRoadIndex < state.routing.activeLegs.length) {
        const step = state.routing.activeLegs[state.drive.currentRoadIndex];
        const stepDist = step.base_length || MIN_STEP_DISTANCE_M;
        const distanceLeftOnStep = stepDist - state.drive.stepProgress;

        const startPos = [step.from_node[1], step.from_node[0]];
        const endPos = [step.to_node[1], step.to_node[0]];

        if (distanceLeftOnStep <= remainingMeters) {
            const spilloverMs = remainingMeters > 0 ? remainingElapsedMs * ((remainingMeters - distanceLeftOnStep) / remainingMeters) : 0;
            const observedMs = Math.max(0, state.sim.currentEdgeTimeMs - spilloverMs);
            remainingMeters -= distanceLeftOnStep;
            remainingElapsedMs = spilloverMs;
            
            if (step.edge_id !== null && step.edge_id !== undefined && observedMs > 0) {
                state.sim.pendingEdgeEvents.push({ edge_id: step.edge_id, observed_sec: observedMs / MS_PER_SECOND });
            }
            state.sim.currentEdgeTimeMs = spilloverMs;
            state.drive.currentRoadIndex++;
            state.drive.stepProgress = 0;
            if (state.drive.currentRoadIndex < state.routing.activeLegs.length) {
                const prevStep = step;
                const nextStep = state.routing.activeLegs[state.drive.currentRoadIndex];
                
                const prevBearing = Math.atan2(prevStep.to_node[1] - prevStep.from_node[1], prevStep.to_node[0] - prevStep.from_node[0]) * DEGREES_PER_RADIAN;
                const nextBearing = Math.atan2(nextStep.to_node[1] - nextStep.from_node[1], nextStep.to_node[0] - nextStep.from_node[0]) * DEGREES_PER_RADIAN;
                
                let diff = Math.abs(nextBearing - prevBearing);
                if (diff > HALF_TURN_DEG) diff = FULL_TURN_DEG - diff;
                
                if (diff > SHARP_TURN_THRESHOLD_DEG && state.sim.motionState) {
                    state.sim.motionState.speedKmh = Math.min(state.sim.motionState.speedKmh, TURN_SPEED_CAP_KMH);
                }
            }
        } else {
            state.drive.stepProgress += remainingMeters;
            const fraction = state.drive.stepProgress / stepDist;
            pos = [startPos[0] + (endPos[0] - startPos[0]) * fraction, startPos[1] + (endPos[1] - startPos[1]) * fraction];
            remainingMeters = 0;
            remainingElapsedMs = 0;
        }
    }

    if (state.drive.isDrifting) {
        state.drive.offRouteOffset[0] += OFF_ROUTE_DRIFT_PER_MS * elapsedMs;
        state.drive.offRouteOffset[1] += OFF_ROUTE_DRIFT_PER_MS * elapsedMs;
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
    return Math.atan2(nextPos[1] - currentPos[1], nextPos[0] - currentPos[0]) * DEGREES_PER_RADIAN;
}
