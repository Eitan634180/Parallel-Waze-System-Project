package config

import "time"

const (
	HTTPClientTimeout               = 5 * time.Second
	RouteCacheGCInterval            = 5 * time.Minute
	SlowRouteRequestLogThreshold    = 150 * time.Millisecond
	SlowSearchRequestLogThreshold   = 300 * time.Millisecond
	SlowSessionCreationLogThreshold = 50 * time.Millisecond
	BaseRouteCount                  = 1
	MaxRouteCount                   = 5
)
