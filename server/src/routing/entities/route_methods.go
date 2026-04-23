package entities

const maxRemainingRouteOffset = 2

func EdgeIDs(steps []Step) []uint32 {
	ids := make([]uint32, 0, len(steps))
	for _, step := range steps {
		if step.EdgeID != nil {
			ids = append(ids, *step.EdgeID)
		}
	}
	return ids
}

func (r Route) InitialStepIndex() int {
	if len(r.Steps) <= 1 {
		return 0
	}
	return 1
}

func (r Route) CurrentEdge(stepIdx int) *uint32 {
	switch {
	case stepIdx < 0 || stepIdx >= len(r.Steps):
		return nil
	case stepIdx == 0:
		if len(r.Steps) < 2 {
			return nil
		}
		return r.Steps[1].EdgeID
	default:
		return r.Steps[stepIdx].EdgeID
	}
}

func (r Route) Destination() (Step, bool) {
	if len(r.Steps) == 0 {
		return Step{}, false
	}
	return r.Steps[len(r.Steps)-1], true
}

func (r Route) SameRemaining(currentStepIdx int, candidate Route) bool {
	if currentStepIdx < 0 {
		currentStepIdx = 0
	}

	currentEdges := remainingEdgeSequence(r, currentStepIdx)
	candidateEdges := remainingEdgeSequence(candidate, candidate.InitialStepIndex())

	if len(candidateEdges) == 0 {
		return true
	}
	if len(candidateEdges) > len(currentEdges) {
		return false
	}

	offset := len(currentEdges) - len(candidateEdges)
	if offset > maxRemainingRouteOffset {
		return false
	}

	for i := range candidateEdges {
		if currentEdges[offset+i] != candidateEdges[i] {
			return false
		}
	}
	return true
}

func (r Route) Patched(repairStepIdx int, patch []Step) Route {
	rawSteps := make([]Step, 0, len(r.Steps)+len(patch))

	for i := 0; i < repairStepIdx; i++ {
		step := r.Steps[i]
		if i == 0 {
			step.DistanceM = 0
			step.BaseTimeSec = 0
		} else {
			step.DistanceM = r.Steps[i].DistanceM - r.Steps[i-1].DistanceM
			step.BaseTimeSec = r.Steps[i].BaseTimeSec - r.Steps[i-1].BaseTimeSec
		}
		rawSteps = append(rawSteps, step)
	}

	rawSteps = append(rawSteps, patch...)

	for i := repairStepIdx + 1; i < len(r.Steps); i++ {
		step := r.Steps[i]
		step.DistanceM = r.Steps[i].DistanceM - r.Steps[i-1].DistanceM
		step.BaseTimeSec = r.Steps[i].BaseTimeSec - r.Steps[i-1].BaseTimeSec
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
		ID:           r.ID,
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
