package config

import "time"

const (
	SimulationTickInterval               = 250 * time.Millisecond
	SimulationObservationWarmupS         = float32(1.0)
	SimulationObservationSampleIntervalS = float32(1.0)
	SimulationRandomSeed                 = int64(42)
	SimulationPaceBiasBase               = float32(0.85)
	SimulationPaceBiasRange              = float32(0.30)
	SimulationMinLegDistanceFallbackM    = float32(0.1)
	SimulationDefaultLegSpeedKmh         = float32(50)
	SimulationMinSpeedKmh                = float32(8)
	SimulationMaxSpeedMultiplier         = float32(1.1)
	SimulationDefaultCount               = 1
	SimulationDefaultRouteAlternatives   = 1
	SimulationMaxCount                   = 1000
	SimulationMinWorkers                 = 1
	SimulationRandomRouteAttemptFactor   = 6
	SimulationMinRandomRouteDistanceSq   = 0.0004
	SimulationMaxCommuteDegrees          = 0.25
	SimulationSlowRouteLogThreshold      = 250 * time.Millisecond
)
