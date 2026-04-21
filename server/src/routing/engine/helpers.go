package engine

func InitialStepIndex(route Route) int {
	if len(route.Steps) <= 1 {
		return 0
	}
	return 1
}

func CurrentEdgeForStep(route Route, stepIdx int) *uint32 {
	switch {
	case stepIdx < 0 || stepIdx >= len(route.Steps):
		return nil
	case stepIdx == 0:
		if len(route.Steps) < 2 {
			return nil
		}
		return route.Steps[1].EdgeID
	default:
		return route.Steps[stepIdx].EdgeID
	}
}

func Destination(route Route) (Step, bool) {
	if len(route.Steps) == 0 {
		return Step{}, false
	}
	return route.Steps[len(route.Steps)-1], true
}

func EdgeIDs(steps []Step) []uint32 {
	ids := make([]uint32, 0, len(steps))
	for _, step := range steps {
		if step.EdgeID != nil {
			ids = append(ids, *step.EdgeID)
		}
	}
	return ids
}

func SameRemainingRoute(current Route, currentStepIdx int, candidate Route) bool {
	if currentStepIdx < 0 {
		currentStepIdx = 0
	}

	currentEdges := remainingEdgeSequence(current, currentStepIdx)
	candidateEdges := remainingEdgeSequence(candidate, InitialStepIndex(candidate))

	if len(candidateEdges) == 0 {
		return true
	}
	if len(candidateEdges) > len(currentEdges) {
		return false
	}

	offset := len(currentEdges) - len(candidateEdges)
	if offset > 2 {
		return false
	}

	for i := range candidateEdges {
		if currentEdges[offset+i] != candidateEdges[i] {
			return false
		}
	}
	return true
}

func RebuildPatchedRoute(oldRoute Route, repairStepIdx int, patch []Step) Route {
	rawSteps := make([]Step, 0, len(oldRoute.Steps)+len(patch))

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

	rawSteps = append(rawSteps, patch...)

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

	return Route{
		ID:           oldRoute.ID,
		Steps:        rawSteps,
		TotalDistM:   totalDistance,
		TotalTimeSec: totalTime,
	}
}

func remainingEdgeSequence(route Route, stepIdx int) []uint32 {
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= len(route.Steps) {
		return nil
	}
	return EdgeIDs(route.Steps[stepIdx:])
}
