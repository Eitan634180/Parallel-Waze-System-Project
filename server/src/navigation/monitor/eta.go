package monitor

import (
	"nav-system/src/navigation/internal/routeutil"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

func ComputeETA(route engine.Route, stepIdx int, lastLat, lastLon float64, g *model.Graph, store *trafficstore.Store) float32 {
	return routeutil.ComputeETA(route, stepIdx, lastLat, lastLon, g, store)
}
