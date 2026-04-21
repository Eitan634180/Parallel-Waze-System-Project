package monitor

import "nav-system/src/routing/engine"

func remainingSteps(route engine.Route, stepIdx int) []engine.Step {
	switch {
	case stepIdx < 0:
		return route.Steps
	case stepIdx >= len(route.Steps):
		return nil
	default:
		return route.Steps[stepIdx:]
	}
}
