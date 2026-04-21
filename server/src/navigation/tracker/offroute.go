package tracker

import (
	navigationoffroute "nav-system/src/navigation/internal/offroute"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
)

const (
	OffRouteDistM      = navigationoffroute.OffRouteDistM
	OffRouteSanityMaxM = navigationoffroute.OffRouteSanityMaxM
)

func DistanceFromExpectedPath(route engine.Route, stepIdx int, lat, lon float64, g *model.Graph) float32 {
	return navigationoffroute.DistanceFromExpectedPath(route, stepIdx, lat, lon, g)
}
