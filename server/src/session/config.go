package session

import (
	"time"

	"nav-system/src/traffic"
)

const (
	sessionExpiry             = 5 * time.Minute
	expiryCheckInterval       = 60 * time.Second
	propagationInterval       = 2 * time.Second
	optimizationSweepInterval = 500 * time.Millisecond
	slowComputeLogThreshold   = 150 * time.Millisecond

	rerouteCooldown = 5 * time.Second
	etaThrottle     = 2 * time.Second

	congestionMultiplier = traffic.CongestionThreshold
	rerouteSpeedupMin    = float32(0.07)
	rerouteMinGainSec    = float32(15)

	offRouteDistM      = float32(50)
	offRouteSanityMaxM = float32(5_000)
	offRouteWindow     = 2
	offRouteStrikes    = 2

	localRepairMaxHops         = 5
	localRepairOriginalMaxHops = 20
	severeCongestionMultiplier = float32(2.0)
	severeCongestionMinDelay   = float32(20.0)

	optimizationWorkerLimit = 10
	maxHeuristicSpeedMps    = 120.0 / 3.6

	rerouteReasonTraffic        = "traffic"
	rerouteReasonLocalPatch     = "local_patch"
	rerouteReasonOffRoute       = "off_route"
	rerouteReasonTrafficCleared = "traffic_cleared"
)
