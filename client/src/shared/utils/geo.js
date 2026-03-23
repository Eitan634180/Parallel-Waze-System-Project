export function flipCoords(geojsonCoords) {
    return geojsonCoords.map((coord) => [coord[1], coord[0]]);
}

export function distanceBetweenLatLngM(lat1, lng1, lat2, lng2) {
    const meanLatRad = ((lat1 + lat2) * 0.5) * Math.PI / 180;
    const metersPerLat = 111320;
    const metersPerLng = 111320 * Math.cos(meanLatRad);
    const dLatM = (lat1 - lat2) * metersPerLat;
    const dLngM = (lng1 - lng2) * metersPerLng;
    return Math.hypot(dLatM, dLngM);
}
