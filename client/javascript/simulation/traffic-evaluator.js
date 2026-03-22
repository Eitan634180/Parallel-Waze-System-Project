import { state } from '../core/state.js';

export function getRouteTrafficStatus(route) {
    if (route?.congestionAhead) {
        return {
            level: route.congestedEdges > 1 ? 'heavy' : 'congested',
            detail: `${route.congestedEdges} congested edge${route.congestedEdges === 1 ? '' : 's'} reported on this route.`,
        };
    }
    return { level: 'normal', detail: 'No slowdowns detected on the selected route.' };
}

export function computeTrafficStatus() {
    if (state.debug.serverData) {
        if (state.debug.serverData.congestion_ahead) {
            return {
                level: state.debug.serverData.congested_edges > 1 ? 'heavy' : 'congested',
                detail: `${state.debug.serverData.congested_edges} congested edge${state.debug.serverData.congested_edges === 1 ? '' : 's'} reported by server.`,
            };
        }
        return { level: 'normal', detail: 'No slowdowns detected on the active route.' };
    }

    const ahead = (state.routing.activeLegs || []).slice(state.drive.currentRoadIndex, state.drive.currentRoadIndex + 6);
    let slowEdges = 0;
    let worstRatio = 1;

    ahead.forEach((step) => {
        if (step?.edge_id === null || step?.edge_id === undefined) return;
        const base = step.speed_limit || 0;
        const recommended = state.sim.recommendedSpeeds.get(step.edge_id);
        if (recommended == null || base <= 0) return;

        const ratio = recommended / base;
        if (ratio < 0.95) {
            slowEdges++;
            worstRatio = Math.min(worstRatio, ratio);
        }
    });

    if (!state.drive.isActive && state.routing.activeObj) {
        return getRouteTrafficStatus(state.routing.activeObj);
    }

    if (slowEdges === 0) {
        return { level: 'normal', detail: 'No slowdowns detected on the active route.' };
    }

    if (worstRatio <= 0.6 || slowEdges >= 2) {
        return { level: 'heavy', detail: `${slowEdges} route edges have heavy slowdowns.` };
    }
    return { level: 'congested', detail: `${slowEdges} route edge has a measurable slowdown.` };
}
