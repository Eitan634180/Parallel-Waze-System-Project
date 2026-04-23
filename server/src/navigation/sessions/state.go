package sessions

import (
	routingentities "nav-system/src/routing/entities"
)

// RemainingSteps returns the steps from the current position to the end.
func (s *Session) RemainingSteps() []routingentities.Step {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return s.remainingStepsLocked()
}

func (s *Session) remainingStepsLocked() []routingentities.Step {
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
	return routingentities.EdgeIDs(s.remainingStepsLocked())
}
