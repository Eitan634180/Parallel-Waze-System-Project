package utilities

const (
	DegreesToRadians       = 3.141592653589793 / 180.0
	EarthRadiusMeters      = 6_371_000.0
	KilometersPerMeter     = 1000.0
	KilometersPerHourToMps = 3.6
	MinLatitude            = -90.0
	MaxLatitude            = 90.0
	MinLongitude           = -180.0
	MaxLongitude           = 180.0
	DefaultReferenceLat    = 31.5
)

func Abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func ClampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func MaxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func MinFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func MinFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func MaxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
