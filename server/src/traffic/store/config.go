package store

const (
	// ewmaAlpha controls how quickly observed travel-time changes affect weight.
	ewmaAlpha = float32(0.15)

	// partialSampleAlpha is used for weaker in-progress speed samples from pings.
	partialSampleAlpha = float32(0.08)

	// CongestionThreshold marks edges that should be treated as congested.
	CongestionThreshold = float32(1.5)

	// SignificantShift is the minimum multiplier change that triggers a speed update.
	SignificantShift = float32(0.10)

	// Hint-speed parameters are only used for speed-update guidance.
	hintJamDensityVehPerKm = float32(120)
	hintMinSpeedRatio      = float32(0.02)
	hintMinEdgeLengthKm    = float32(0.02)
	hintAlpha              = 1.0

	storeGrowthMultiplier = 2
	storeGrowthPadding    = 1024
)
