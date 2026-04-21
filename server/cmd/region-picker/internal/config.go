package picker

import (
	"time"

	"nav-system/src/utilities"
)

const (
	indexMaxAge          = 168 * time.Hour
	indexRequestTimeout  = 30 * time.Second
	serverSearchMaxDepth = 6
)

func DefaultGeofabrikCacheFile() string {
	return utilities.RequireEnv("NAV_GEOFABRIK_CACHE_FILE")
}

func GeofabrikIndexURL() string {
	return utilities.RequireEnv("NAV_GEOFABRIK_INDEX_URL")
}
