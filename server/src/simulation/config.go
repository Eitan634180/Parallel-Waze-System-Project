package simulation

import "time"

const (
	TickInterval               = 250 * time.Millisecond
	MaxCatchUpSteps            = 4
	TickWorkChunkSize          = 64
	ObservationWarmupS         = float32(1.0)
	ObservationSampleIntervalS = float32(1.0)
	RandomSeed                 = int64(42)
	PaceBiasBase               = float32(0.85)
	PaceBiasRange              = float32(0.30)
	MinLegDistanceFallbackM    = float32(0.1)
	DefaultLegSpeedKmh         = float32(50)
	MinSpeedKmh                = float32(8)
	MaxSpeedMultiplier         = float32(1.1)
	DefaultCount               = 1
	DefaultRouteAlternatives   = 1
	MaxCount                   = 10000
	MinWorkers                 = 1
	RandomRouteAttemptFactor   = 6
	MinRandomRouteDistanceSq   = 0.0004
	MaxCommuteDegrees          = 0.25
	SlowRouteLogThreshold      = 250 * time.Millisecond
)
