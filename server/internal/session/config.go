package session

import (
	"time"

	"nav-system/internal/traffic"
)

const (
	sessionExpiry             = 5 * time.Minute
	expiryCheckInterval       = 60 * time.Second
	propagationInterval       = 2 * time.Second
	optimizationSweepInterval = 2 * time.Second

	rerouteCooldown = 5 * time.Second
	etaThrottle     = 2 * time.Second

	congestionMultiplier = traffic.CongestionThreshold
	rerouteSpeedupMin    = float32(0.10)
	rerouteMinGainSec    = float32(30)

	offRouteDistM      = float32(50)
	offRouteSanityMaxM = float32(5_000)
	offRouteWindow     = 2
	offRouteStrikes    = 2

	localRepairMaxHops         = 5
	severeCongestionMultiplier = float32(2.0)
	severeCongestionMinDelay   = float32(20.0)

	optimizationWorkerLimit = 10
	maxHeuristicSpeedMps    = 120.0 / 3.6
)
