import { DEFAULT_SPEED_LIMIT } from "./config.js"

export function formatDuration(seconds) {
    if (!seconds || seconds < 0) return '--';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (hrs > 0) return `${hrs}h ${mins}m`;
    return `${mins} min`;
}

export function getArrivalTime(secondsFromNow) {
    const arrivalDate = new Date(Date.now() + secondsFromNow * 1000);
    const hours = arrivalDate.getHours().toString().padStart(2, '0');
    const mins = arrivalDate.getMinutes().toString().padStart(2, '0');
    return `${hours}:${mins}`;
}

export function flipCoords(geojsonCoords) {
    return geojsonCoords.map(c => [c[1], c[0]]);
}

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
        pathCoords: steps.map(step => [step.lon, step.lat]),
        distance: route.total_dist_m || 0,
        dynamicETA: route.total_time_sec || 0,
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
        const denom = dLat * dLat + dLng * dLng;
        const rawT = denom > 0 ? (((lat - startLat) * dLat) + ((lng - startLng) * dLng)) / denom : 0;
        const t = Math.max(0, Math.min(1, rawT));
        const projLat = startLat + dLat * t;
        const projLng = startLng + dLng * t;
        const distSq = ((lat - projLat) ** 2) + ((lng - projLng) ** 2);
        const offsetM = distanceBetweenLatLngM(lat, lng, projLat, projLng);
        const progressM = (leg.base_length || 0) * t;
        const startDistance = routeObj.steps[index]?.distance_m || 0;

        if (!best || distSq < best.distSq) {
            best = {
                distSq,
                offsetM,
                lat: projLat,
                lng: projLng,
                roadIndex: index,
                stepProgress: progressM,
                distanceLeft: Math.max(0, (routeObj.distance || 0) - startDistance - progressM),
            };
        }
    });

    return best;
}

export function distanceBetweenLatLngM(lat1, lng1, lat2, lng2) {
    const meanLatRad = ((lat1 + lat2) * 0.5) * Math.PI / 180;
    const metersPerLat = 111320;
    const metersPerLng = 111320 * Math.cos(meanLatRad);
    const dLatM = (lat1 - lat2) * metersPerLat;
    const dLngM = (lng1 - lng2) * metersPerLng;
    return Math.hypot(dLatM, dLngM);
}
