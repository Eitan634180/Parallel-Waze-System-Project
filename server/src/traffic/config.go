package traffic

import "time"

const (
	// How quickly observed travel time changes affect weight.
	EwmaAlpha = float32(0.15)

	// How quickly observed speed samples affect weight.
	PartialSampleAlpha = float32(0.08)

	// When edges should be treated as congested.
	CongestionThreshold = float32(1.5)

	// Minimum multiplier change that triggers a speed update.
	SignificantShift = float32(0.10)

	// Hint-speed parameters are only used for speed-update guidance.
	HintJamDensityVehPerKm = float32(120)
	HintMinSpeedRatio      = float32(0.02)
	HintMinEdgeLengthKm    = float32(0.02)
	HintAlpha              = 1.0

	StoreGrowthMultiplier = 2
	StoreGrowthPadding    = 1024

	CustomizationInterval         = 5 * time.Second
	SlowCustomizationLogThreshold = 500 * time.Millisecond

	DecayInterval  = 2 * time.Second
	DecayFactor    = float32(0.5)
	DecayTolerance = float32(0.05)

	FullAffectedCellsDirtyCoverage = 0.25
)
