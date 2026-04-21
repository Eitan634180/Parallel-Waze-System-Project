package navigation

import "time"

const (
	SessionExpiry              = 5 * time.Minute
	SessionExpiryCheckInterval = 60 * time.Second

	PropagationInterval       = 2 * time.Second
	OptimizationSweepInterval = 500 * time.Millisecond
	SlowComputeLogThreshold   = 150 * time.Millisecond
	RerouteCooldown           = 10 * time.Second
	ETAThrottle               = 2 * time.Second
	RerouteSpeedupMin         = float32(0.07)
	RerouteMinGainSec         = float32(15)
	OffRouteStrikes           = 2
	OptimizationWorkerLimit   = 10
	MaxHeuristicSpeedMps      = 120.0 / 3.6
	PropagationJobQueueFactor = 4

	LocalRepairMaxHops         = 5
	LocalRepairOriginalMaxHops = 20
	SevereCongestionMultiplier = float32(2.0)
	SevereCongestionMinDelay   = float32(20.0)

	RerouteReasonTraffic        = "traffic"
	RerouteReasonLocalPatch     = "local_patch"
	RerouteReasonOffRoute       = "off_route"
	RerouteReasonTrafficCleared = "traffic_cleared"
)
