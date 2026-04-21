package config

import "time"

const (
	RoutingPropagationInterval       = 2 * time.Second
	RoutingOptimizationSweepInterval = 500 * time.Millisecond
	RoutingSlowComputeLogThreshold   = 150 * time.Millisecond
	RoutingRerouteCooldown           = 10 * time.Second
	RoutingETAThrottle               = 2 * time.Second
	RoutingRerouteSpeedupMin         = float32(0.07)
	RoutingRerouteMinGainSec         = float32(15)
	RoutingOffRouteStrikes           = 2
	RoutingOptimizationWorkerLimit   = 10
	RoutingMaxHeuristicSpeedMps      = 120.0 / 3.6
	RoutingPropagationJobQueueFactor = 4
)
