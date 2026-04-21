package config

import "time"

const (
	APIHTTPClientTimeout           = 5 * time.Second
	APIRouteCacheGCInterval        = 5 * time.Minute
	APISlowRouteRequestLogThreshold = 150 * time.Millisecond
	APISlowSearchRequestLogThreshold = 300 * time.Millisecond
	APISlowSessionCreationLogThreshold = 50 * time.Millisecond
	APIBaseRouteCount = 1
	APIMaxRouteCount  = 5
)
