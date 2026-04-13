import { DEFAULT_SPEED_LIMIT } from '../../app/app-config.js';
import { distanceBetweenLatLngM } from '../../utils/geo.js';

export function processRawRoute(route) {
    const steps = route.steps || [];
    const segments = [];

    for (let i = 1; i < steps.length; i++) {
        const prev = steps[i - 1];
        const current = steps[i];
        const distance = Math.max(0, (current.distance_m || 0) - (prev.distance_m || 0));
        const duration = Math.max(0, (current.base_time_sec || 0) - (prev.base_time_sec || 0));
        const speedMs = duration > 0 ? distance / duration : DEFAULT_SPEED_LIMIT;

        segments.push({
            from_node: [prev.lon, prev.lat],
            to_node: [current.lon, current.lat],
            base_length: distance,
            duration_sec: duration,
            speed_limit: Math.round(speedMs * 3.6),
            edge_id: current.edge_id ?? null,
        });
    }

    return {
        id: route.id,
        steps,
        legs: segments,
        pathCoords: steps.map((step) => [step.lon, step.lat]),
        distance: route.total_dist_m || 0,
        dynamicETA: route.total_time_sec || 0,
        congestionAhead: Boolean(route.congestion_ahead),
        congestedEdges: route.congested_edges || 0,
    };
}

export function projectPositionOntoRoute(routeObj, lat, lng) {
    if (!routeObj?.legs?.length) {
        return null;
    }

    let best = null;

    routeObj.legs.forEach((leg, index) => {
        const startLat = leg.from_node[1];
        const startLng = leg.from_node[0];
        const endLat = leg.to_node[1];
        const endLng = leg.to_node[0];
        const dLat = endLat - startLat;
        const dLng = endLng - startLng;
        const denominator = dLat * dLat + dLng * dLng;
        const rawT = denominator > 0
            ? (((lat - startLat) * dLat) + ((lng - startLng) * dLng)) / denominator
            : 0;
        const t = Math.max(0, Math.min(1, rawT));
        const projectedLat = startLat + dLat * t;
        const projectedLng = startLng + dLng * t;
        const distSq = ((lat - projectedLat) ** 2) + ((lng - projectedLng) ** 2);
        const offsetM = distanceBetweenLatLngM(lat, lng, projectedLat, projectedLng);
        const progressM = (leg.base_length || 0) * t;
        const startDistance = routeObj.steps[index]?.distance_m || 0;

        if (!best || distSq < best.distSq) {
            best = {
                distSq,
                offsetM,
                lat: projectedLat,
                lng: projectedLng,
                roadIndex: index,
                stepProgress: progressM,
                distanceLeft: Math.max(0, (routeObj.distance || 0) - startDistance - progressM),
                distanceLeftOnStep: Math.max(0, (leg.base_length || 0) - progressM),
            };
        }
    });

    return best;
}
