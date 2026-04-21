package config

import "time"

const (
	TrafficCustomizationInterval         = 5 * time.Second
	TrafficSlowCustomizationLogThreshold = 500 * time.Millisecond
	TrafficDecayInterval                 = 2 * time.Second
	TrafficDecayFactor                   = float32(0.5)
	TrafficDecayTolerance                = float32(0.05)
)
