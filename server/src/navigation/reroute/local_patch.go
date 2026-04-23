package reroute

import (
	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationsessions "nav-system/src/navigation/sessions"
	trafficstore "nav-system/src/traffic/store"
)

func checkLocalRepairTriggerLocked(s *navigationsessions.Session, store *trafficstore.Store, g *model.Graph) (bool, int, float32, bool) {
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
		edge, ok := g.Edge(edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.BaseWeight)
		if liveWeight < edge.BaseWeight*navigation.SevereCongestionMultiplier || liveWeight-edge.BaseWeight < navigation.SevereCongestionMinDelay {
			continue
		}

		idxU := s.Route.Steps[i-1].NodeIdx
		idxV := s.Route.Steps[i].NodeIdx
		u := g.Node(idxU)
		v := g.Node(idxV)
		if u != nil && v != nil {
			isCrossCell := u.CellID != v.CellID
			return true, i, liveWeight, isCrossCell
		}
	}

	return false, -1, 0, false
}
