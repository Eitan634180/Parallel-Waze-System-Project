package monitor

import (
	"nav-system/src/navigation/internal/routeutil"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

func RemainingCongestionSummary(route engine.Route, stepIdx int, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return routeutil.RemainingCongestionSummary(route, stepIdx, store, g)
}

func RouteCongestionSummary(route engine.Route, store *trafficstore.Store, g *model.Graph) (bool, int) {
	return routeutil.RouteCongestionSummary(route, store, g)
}
