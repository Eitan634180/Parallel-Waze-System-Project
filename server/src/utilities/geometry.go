package utilities

import "math"

const (
	EarthRadiusM = float32(EarthRadiusMeters)
	ReferenceLat = DefaultReferenceLat
)

var cosReferenceLat = float32(math.Cos(ReferenceLat * DegreesToRadians))

// Project converts geographic coordinates into planar metres using a default ReferenceLat.
func Project(lat, lon float64) (x, y float32) {
	return ProjectAtReferenceLat(lat, lon, ReferenceLat)
}

// Convert (lat, lon) to (x, y) with using an equirectangular projection anchored at the supplied reference latitude.
// Note: Earth is not round so there is a small error when converting to flat coordinates!
func ProjectAtReferenceLat(lat, lon, referenceLat float64) (x, y float32) {
	cosRefLat := float32(math.Cos(referenceLat * DegreesToRadians))
	x = float32(lon*DegreesToRadians) * cosRefLat * EarthRadiusM
	y = float32(lat*DegreesToRadians) * EarthRadiusM
	return
}

// HaversineM returns the great-circle distance in metres between two WGS-84 coords.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := (lat2 - lat1) * DegreesToRadians
	dLon := (lon2 - lon1) * DegreesToRadians
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*DegreesToRadians)*math.Cos(lat2*DegreesToRadians)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return float64(EarthRadiusM) * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func Distance(ax, ay, bx, by float32) float32 {
	return float32(math.Sqrt(float64(DistanceSquared(ax, ay, bx, by))))
}

func DistanceSquared(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func DistancePointToSegment(px, py, ax, ay, bx, by float32) float32 {
	abx := bx - ax
	aby := by - ay
	denominator := abx*abx + aby*aby
	if denominator == 0 {
		return Distance(px, py, ax, ay)
	}

	t := ((px-ax)*abx + (py-ay)*aby) / denominator
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	closestX := ax + t*abx
	closestY := ay + t*aby
	return Distance(px, py, closestX, closestY)
}
