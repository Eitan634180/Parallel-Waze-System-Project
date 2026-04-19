package session

import (
	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/traffic"
)

func checkLocalRepairTriggerLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int, float32, bool) {
	for i := maxInt(1, s.StepIdx); i < len(s.Route.Steps); i++ {
		step := s.Route.Steps[i]
		if step.EdgeID == nil {
			continue
		}

		edgeID := builder.EdgeID(*step.EdgeID)
		edge, ok := graphEdge(g, edgeID)
		if !ok {
			continue
		}

		liveWeight := store.LiveWeight(edgeID, edge.Weight)
		if liveWeight < edge.Weight*severeCongestionMultiplier || liveWeight-edge.Weight < severeCongestionMinDelay {
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

func rebuildPatchedRoute(oldRoute routing.Route, repairStepIdx int, patch []routing.Step) routing.Route {
	rawSteps := make([]routing.Step, 0, len(oldRoute.Steps)+len(patch))

	// Edges before repair index
	for i := 0; i < repairStepIdx; i++ {
		step := oldRoute.Steps[i]
		if i == 0 {
			step.DistanceM = 0
			step.BaseTimeSec = 0
		} else {
			step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
			step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		}
		rawSteps = append(rawSteps, step)
	}

	// Patch at repair index
	rawSteps = append(rawSteps, patch...)

	// Edges after repair index
	for i := repairStepIdx + 1; i < len(oldRoute.Steps); i++ {
		step := oldRoute.Steps[i]
		step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
		step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		rawSteps = append(rawSteps, step)
	}

	var totalDistance float32
	var totalTime float32
	for i := range rawSteps {
		if i > 0 {
			totalDistance += rawSteps[i].DistanceM
			totalTime += rawSteps[i].BaseTimeSec
		}
		rawSteps[i].DistanceM = totalDistance
		rawSteps[i].BaseTimeSec = totalTime
	}

	return routing.Route{
		ID:           oldRoute.ID,
		Steps:        rawSteps,
		TotalDistM:   totalDistance,
		TotalTimeSec: totalTime,
	}
}


