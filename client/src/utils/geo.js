import { METERS_PER_DEGREE_LATITUDE, RADIANS_PER_DEGREE } from './math.js';

const GEO = {
    meanFactor: 0.5,
};

export function flipCoords(geojsonCoords) {
    return geojsonCoords.map((coord) => [coord[1], coord[0]]);
}

export function distanceBetweenLatLngM(lat1, lng1, lat2, lng2) {
    const meanLatRad = ((lat1 + lat2) * GEO.meanFactor) * RADIANS_PER_DEGREE;
    const metersPerLat = METERS_PER_DEGREE_LATITUDE;
    const metersPerLng = METERS_PER_DEGREE_LATITUDE * Math.cos(meanLatRad);
    const dLatM = (lat1 - lat2) * metersPerLat;
    const dLngM = (lng1 - lng2) * metersPerLng;
    return Math.hypot(dLatM, dLngM);
}
