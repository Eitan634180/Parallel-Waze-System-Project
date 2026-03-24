package session

import (
	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

func checkLocalRepairTriggerLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int, float32) {
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

		u := g.NodeByID(s.Route.Steps[i-1].NodeID)
		v := g.NodeByID(s.Route.Steps[i].NodeID)
		if u != nil && v != nil && u.CellID != v.CellID {
			return true, i, liveWeight
		}
	}

	return false, -1, 0
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

func cloneRoute(route routing.Route) routing.Route {
	cloned := route
	cloned.Steps = append([]routing.Step(nil), route.Steps...)
	return cloned
}
