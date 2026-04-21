package routing

import "nav-system/src/utilities"

const (
	MaxSearchSpeedMps       = 120.0 / utilities.KilometersPerHourToMps
	AlternativeRoutePenalty = float32(5.0)
)
