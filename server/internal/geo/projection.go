package geo

import "math"

const (
	EarthRadiusM = float32(6_371_000.0)
	ReferenceLat = 31.5
)

var cosReferenceLat = float32(math.Cos(ReferenceLat * math.Pi / 180.0))

// Project converts geographic coordinates into planar metres using the same
// projection the graph builder uses.
func Project(lat, lon float64) (x, y float32) {
	x = float32(lon*math.Pi/180.0) * cosReferenceLat * EarthRadiusM
	y = float32(lat*math.Pi/180.0) * EarthRadiusM
	return
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
