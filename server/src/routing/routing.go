package routing

import (
	"nav-system/src/graph/geography"
	"nav-system/src/graph/model"
	"nav-system/src/routing/engine"
)

type (
	WeightFunc  = engine.WeightFunc
	Router      = engine.Router
	SnapIndex   = geography.SnapIndex
	Step        = engine.Step
	Route       = engine.Route
	RoutingMode = engine.RoutingMode
	SearchStats = engine.SearchStats
)

const (
	RoutingModeHierarchical = engine.RoutingModeHierarchical
	RoutingModeBaseAStar    = engine.RoutingModeBaseAStar
	RoutingModeBaseDijkstra = engine.RoutingModeBaseDijkstra
)

var BaseWeight = engine.BaseWeight

func ParseRoutingMode(raw string) (RoutingMode, error) {
	return engine.ParseRoutingMode(raw)
}

func NewRouter(g *model.Graph, si *SnapIndex) *Router {
	return engine.NewRouter(g, si)
}

func NewRouterWithMode(g *model.Graph, si *SnapIndex, mode RoutingMode) *Router {
	return engine.NewRouterWithMode(g, si, mode)
}

func BuildSnapIndex(g *model.Graph) *SnapIndex {
	return geography.BuildSnapIndex(g)
}
