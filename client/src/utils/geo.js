const GEO = {
    degreesToRadians: Math.PI / 180,
    metersPerDegreeLatitude: 111320,
    meanFactor: 0.5,
};

export function flipCoords(geojsonCoords) {
    return geojsonCoords.map((coord) => [coord[1], coord[0]]);
}

export function distanceBetweenLatLngM(lat1, lng1, lat2, lng2) {
    const meanLatRad = ((lat1 + lat2) * GEO.meanFactor) * GEO.degreesToRadians;
    const metersPerLat = GEO.metersPerDegreeLatitude;
    const metersPerLng = GEO.metersPerDegreeLatitude * Math.cos(meanLatRad);
    const dLatM = (lat1 - lat2) * metersPerLat;
    const dLngM = (lng1 - lng2) * metersPerLng;
    return Math.hypot(dLatM, dLngM);
}
