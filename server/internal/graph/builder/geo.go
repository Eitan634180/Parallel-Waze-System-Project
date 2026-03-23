package builder

import "math"

const (
	earthRadiusM = 6_371_000.0
	// lat0Deg anchors the equirectangular projection near the map center.
	lat0Deg = 31.5
)

var cosLat0 = math.Cos(lat0Deg * math.Pi / 180.0)

// ProjectXY converts WGS-84 coordinates into planar metres.
func ProjectXY(lat, lon float64) (x, y float32) {
	x = float32(lon * math.Pi / 180.0 * cosLat0 * earthRadiusM)
	y = float32(lat * math.Pi / 180.0 * earthRadiusM)
	return
}

// HaversineM returns the great-circle distance in metres between two WGS-84 coords.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = earthRadiusM
	rad := math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
