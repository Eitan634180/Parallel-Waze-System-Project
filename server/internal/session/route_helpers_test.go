package session

import (
	"testing"

	"nav-system/internal/routing"
)

func TestSameRemainingRouteAllowsShortSuffixMatch(t *testing.T) {
	current := routing.Route{
		Steps: []routing.Step{
			{NodeID: 1},
			{NodeID: 2, EdgeID: edgeID(1)},
			{NodeID: 3, EdgeID: edgeID(2)},
			{NodeID: 4, EdgeID: edgeID(3)},
			{NodeID: 5, EdgeID: edgeID(4)},
		},
	}
	candidate := routing.Route{
		Steps: []routing.Step{
			{NodeID: 9},
			{NodeID: 4, EdgeID: edgeID(3)},
			{NodeID: 5, EdgeID: edgeID(4)},
		},
	}

	session := &Session{Route: current, StepIdx: 1}
	if !sameRemainingRoute(session, candidate) {
		t.Fatal("expected candidate suffix to be treated as the same remaining route")
	}
}

func TestRebuildPatchedRouteRecomputesCumulativeMetrics(t *testing.T) {
	oldRoute := routing.Route{
		ID: "route-1",
		Steps: []routing.Step{
			{NodeID: 1, DistanceM: 0, BaseTimeSec: 0},
			{NodeID: 2, EdgeID: edgeID(10), DistanceM: 100, BaseTimeSec: 10},
			{NodeID: 3, EdgeID: edgeID(11), DistanceM: 200, BaseTimeSec: 30},
			{NodeID: 4, EdgeID: edgeID(12), DistanceM: 350, BaseTimeSec: 50},
		},
	}
	patch := []routing.Step{
		{NodeID: 20, EdgeID: edgeID(30), DistanceM: 50, BaseTimeSec: 8},
		{NodeID: 3, EdgeID: edgeID(31), DistanceM: 40, BaseTimeSec: 8},
	}

	rebuilt := rebuildPatchedRoute(oldRoute, 2, patch)
	if len(rebuilt.Steps) != 5 {
		t.Fatalf("expected 5 steps after patch, got %d", len(rebuilt.Steps))
	}

	wantDistances := []float32{0, 100, 150, 190, 340}
	wantTimes := []float32{0, 10, 18, 26, 46}
	for i := range rebuilt.Steps {
		if rebuilt.Steps[i].DistanceM != wantDistances[i] {
			t.Fatalf("step %d distance = %v, want %v", i, rebuilt.Steps[i].DistanceM, wantDistances[i])
		}
		if rebuilt.Steps[i].BaseTimeSec != wantTimes[i] {
			t.Fatalf("step %d time = %v, want %v", i, rebuilt.Steps[i].BaseTimeSec, wantTimes[i])
		}
	}
}

func edgeID(id uint32) *uint32 {
	return &id
}
