package config

import "time"

const (
	SessionExpiry              = 5 * time.Minute
	SessionExpiryCheckInterval = 60 * time.Second
)
