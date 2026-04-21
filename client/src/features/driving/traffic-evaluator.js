import { state } from '../../app/app-state.js';
import { PANEL_TEXT, TRAFFIC_LEVELS } from '../../ui/ui-constants.js';
import { TRAFFIC_EVALUATION } from './config.js';

export function getRouteTrafficStatus(route) {
    if (route?.congestionAhead) {
        return {
            level: route.congestedEdges > 1 ? TRAFFIC_LEVELS.heavy : TRAFFIC_LEVELS.congested,
            detail: `${route.congestedEdges} congested edge${route.congestedEdges === 1 ? '' : 's'} reported on this route.`,
        };
    }
    return { level: TRAFFIC_LEVELS.normal, detail: 'No slowdowns detected on the selected route.' };
}

export function computeTrafficStatus() {
    if (state.debug.serverData) {
        if (state.debug.serverData.congestion_ahead) {
            return {
                level: state.debug.serverData.congested_edges > 1 ? TRAFFIC_LEVELS.heavy : TRAFFIC_LEVELS.congested,
                detail: `${state.debug.serverData.congested_edges} congested edge${state.debug.serverData.congested_edges === 1 ? '' : 's'} reported by server.`,
            };
        }
        return { level: TRAFFIC_LEVELS.normal, detail: PANEL_TEXT.activeRouteClear };
    }

    const ahead = (state.routing.activeLegs || []).slice(
        state.drive.currentRoadIndex,
        state.drive.currentRoadIndex + TRAFFIC_EVALUATION.aheadWindowSize,
    );
    let slowEdges = 0;
    let worstRatio = 1;

    ahead.forEach((step) => {
        if (step?.edge_id === null || step?.edge_id === undefined) return;
        const base = step.speed_limit || 0;
        const recommended = state.sim.recommendedSpeeds.get(step.edge_id);
        if (recommended == null || base <= 0) return;

        const ratio = recommended / base;
        if (ratio < TRAFFIC_EVALUATION.slowdownRatio) {
            slowEdges++;
            worstRatio = Math.min(worstRatio, ratio);
        }
    });

    if (!state.drive.isActive && state.routing.activeObj) {
        return getRouteTrafficStatus(state.routing.activeObj);
    }

    if (slowEdges === 0) {
        return { level: TRAFFIC_LEVELS.normal, detail: PANEL_TEXT.activeRouteClear };
    }

    if (worstRatio <= TRAFFIC_EVALUATION.heavyWorstRatio || slowEdges >= TRAFFIC_EVALUATION.heavySlowEdgeCount) {
        return { level: TRAFFIC_LEVELS.heavy, detail: `${slowEdges} route edges have heavy slowdowns.` };
    }
    return { level: TRAFFIC_LEVELS.congested, detail: `${slowEdges} route edge has a measurable slowdown.` };
}
