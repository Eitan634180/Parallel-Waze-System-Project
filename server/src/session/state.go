package session

import "nav-system/src/routing"

// RemainingSteps returns the steps from the current position to the end.
func (s *Session) RemainingSteps() []routing.Step {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.remainingStepsLocked()
}

func (s *Session) remainingStepsLocked() []routing.Step {
	switch {
	case s.StepIdx < 0:
		return s.Route.Steps
	case s.StepIdx >= len(s.Route.Steps):
		return nil
	default:
		return s.Route.Steps[s.StepIdx:]
	}
}

// RemainingEdges returns the EdgeIDs of all remaining steps.
func (s *Session) RemainingEdges() []uint32 {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return stepEdgeIDs(s.remainingStepsLocked())
}

func InitialStepIndex(route routing.Route) int {
	if len(route.Steps) <= 1 {
		return 0
	}
	return 1
}

func CurrentEdgeForStep(route routing.Route, stepIdx int) *uint32 {
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

func routeDestination(route routing.Route) (routing.Step, bool) {
	if len(route.Steps) == 0 {
		return routing.Step{}, false
	}
	return route.Steps[len(route.Steps)-1], true
}

func stepEdgeIDs(steps []routing.Step) []uint32 {
	ids := make([]uint32, 0, len(steps))
	for _, step := range steps {
		if step.EdgeID != nil {
			ids = append(ids, *step.EdgeID)
		}
	}
	return ids
}
