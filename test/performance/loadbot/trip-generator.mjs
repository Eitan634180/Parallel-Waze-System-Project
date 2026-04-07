const minTripDistanceSq = 0.0004;
const maxTripAttempts = 10;

export async function fetchBBox(serverURL) {
  const response = await fetch(`${serverURL}/system/info`);
  if (!response.ok) {
    throw new Error(`system info request failed with ${response.status}`);
  }

  const payload = await response.json();
  if (!payload?.bbox) {
    throw new Error('system info did not include bbox');
  }
  return payload.bbox;
}

export function createTripGenerator(bbox, insetFraction, commuteDegrees) {
  const tripBBox = insetBBox(bbox, insetFraction);
  return () => randomTrip(tripBBox, commuteDegrees);
}

function randomTrip(currentBBox, commuteDegrees) {
  const source = randomPoint(currentBBox, 'Loadbot Source');
  let dest = randomNearbyPoint(currentBBox, source, commuteDegrees, 'Loadbot Destination');

  for (let attempt = 0; attempt < maxTripAttempts; attempt += 1) {
    if (distanceSquared(source.lat, source.lng, dest.lat, dest.lng) >= minTripDistanceSq) {
      return { source, dest };
    }
    dest = randomNearbyPoint(currentBBox, source, commuteDegrees, 'Loadbot Destination');
  }

  return { source, dest };
}

function insetBBox(currentBBox, fraction) {
  const latInset = (currentBBox.max_lat - currentBBox.min_lat) * fraction;
  const lonInset = (currentBBox.max_lon - currentBBox.min_lon) * fraction;
  const minLat = currentBBox.min_lat + latInset;
  const maxLat = currentBBox.max_lat - latInset;
  const minLon = currentBBox.min_lon + lonInset;
  const maxLon = currentBBox.max_lon - lonInset;

  if (minLat >= maxLat || minLon >= maxLon) {
    return currentBBox;
  }

  return {
    min_lat: minLat,
    max_lat: maxLat,
    min_lon: minLon,
    max_lon: maxLon,
  };
}

function randomPoint(currentBBox, name) {
  return {
    lat: randomBetween(currentBBox.min_lat, currentBBox.max_lat),
    lng: randomBetween(currentBBox.min_lon, currentBBox.max_lon),
    name,
  };
}

function randomNearbyPoint(currentBBox, source, commuteDegrees, name) {
  const minLat = Math.max(currentBBox.min_lat, source.lat - commuteDegrees);
  const maxLat = Math.min(currentBBox.max_lat, source.lat + commuteDegrees);
  const minLon = Math.max(currentBBox.min_lon, source.lng - commuteDegrees);
  const maxLon = Math.min(currentBBox.max_lon, source.lng + commuteDegrees);

  return {
    lat: randomBetween(minLat, maxLat),
    lng: randomBetween(minLon, maxLon),
    name,
  };
}

function randomBetween(min, max) {
  return min + Math.random() * (max - min);
}

function distanceSquared(ax, ay, bx, by) {
  const dx = ax - bx;
  const dy = ay - by;
  return dx * dx + dy * dy;
}
