package traffic

import "time"

const (
	CustomizationInterval         = 5 * time.Second
	SlowCustomizationLogThreshold = 500 * time.Millisecond
	DecayInterval                 = 2 * time.Second
	DecayFactor                   = float32(0.5)
	DecayTolerance                = float32(0.05)
)
