package sessions

import (
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
)

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
	return engine.EdgeIDs(s.remainingStepsLocked())
}
