package reroute

import (
	coreconfig "nav-system/src/core/config"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/graph/model"
	routinggeometry "nav-system/src/routing/geometry"
	trafficstore "nav-system/src/traffic/store"
)

func checkLocalRepairTriggerLocked(s *navigationsession.Session, store *trafficstore.Store, g *model.Graph) (bool, int, float32, bool) {
	start := s.StepIdx
	if start < 1 {
		start = 1
	}

	for i := start; i < len(s.Route.Steps); i++ {
		step := s.Route.Steps[i]
		if step.EdgeID == nil {
			continue
		}

		edgeID := model.EdgeID(*step.EdgeID)
		edge, ok := routinggeometry.EdgeByID(g, edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.Weight)
		if liveWeight < edge.Weight*coreconfig.RoutingSevereCongestionMultiplier || liveWeight-edge.Weight < coreconfig.RoutingSevereCongestionMinDelay {
			continue
		}

		idxU := s.Route.Steps[i-1].NodeIdx
		idxV := s.Route.Steps[i].NodeIdx
		if idxU < uint32(len(g.Nodes)) && idxV < uint32(len(g.Nodes)) {
			u := &g.Nodes[idxU]
			v := &g.Nodes[idxV]
			isCrossCell := u.CellID != v.CellID
			return true, i, liveWeight, isCrossCell
		}
	}

	return false, -1, 0, false
}
