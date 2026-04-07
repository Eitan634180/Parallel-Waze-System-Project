package testutil

import (
	"math"
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
)

const FloatTolerance = float32(0.001)

type ValidatedRoute struct {
	EdgeIDs   []builder.EdgeID
	Cost      float32
	Distance  float32
	SourceID  builder.NodeID
	TargetID  builder.NodeID
	FinalStep routing.Step
}

func AssertRouteValid(tb testing.TB, g *builder.Graph, route routing.Route, wf routing.WeightFunc) ValidatedRoute {
	tb.Helper()

	if wf == nil {
		wf = routing.BaseWeight
	}
	if len(route.Steps) < 2 {
		tb.Fatalf("route must contain at least 2 steps, got %d", len(route.Steps))
	}
	if route.Steps[0].EdgeID != nil {
		tb.Fatalf("first step must not have an incoming edge")
	}

	var cumulativeDistance float32
	var cumulativeCost float32
	edgeIDs := make([]builder.EdgeID, 0, len(route.Steps)-1)

	for i := 1; i < len(route.Steps); i++ {
		prev := route.Steps[i-1]
		cur := route.Steps[i]
		if cur.EdgeID == nil {
			tb.Fatalf("step %d missing edge id", i)
		}

		edgeID := builder.EdgeID(*cur.EdgeID)
		if int(edgeID) >= len(g.Edges) {
			tb.Fatalf("step %d edge %d is out of range", i, edgeID)
		}
		edge := &g.Edges[edgeID]
		if edge.FromNodeID != prev.NodeID || edge.ToNodeID != cur.NodeID {
			tb.Fatalf("step %d edge %d does not connect %d -> %d", i, edgeID, prev.NodeID, cur.NodeID)
		}

		cumulativeDistance += edge.DistanceM
		cumulativeCost += wf(edge)
		edgeIDs = append(edgeIDs, edgeID)

		assertApprox32(tb, cur.DistanceM, cumulativeDistance, "step distance mismatch")
		assertApprox32(tb, cur.BaseTimeSec, cumulativeCost, "step time mismatch")

		if cur.DistanceM < prev.DistanceM || cur.BaseTimeSec < prev.BaseTimeSec {
			tb.Fatalf("step %d cumulative metrics must be monotonic", i)
		}
	}

	assertApprox32(tb, route.TotalDistM, cumulativeDistance, "route total distance mismatch")
	assertApprox32(tb, route.TotalTimeSec, cumulativeCost, "route total time mismatch")

	return ValidatedRoute{
		EdgeIDs:   edgeIDs,
		Cost:      cumulativeCost,
		Distance:  cumulativeDistance,
		SourceID:  route.Steps[0].NodeID,
		TargetID:  route.Steps[len(route.Steps)-1].NodeID,
		FinalStep: route.Steps[len(route.Steps)-1],
	}
}

func AssertRouteMatchesOracle(tb testing.TB, g *builder.Graph, route routing.Route, oracle OraclePath, wf routing.WeightFunc) {
	tb.Helper()

	validated := AssertRouteValid(tb, g, route, wf)
	if len(oracle.NodeIdxs) == 0 {
		tb.Fatalf("oracle path is empty")
	}

	if validated.SourceID != g.Nodes[oracle.NodeIdxs[0]].ID {
		tb.Fatalf("route source %d does not match oracle source %d", validated.SourceID, g.Nodes[oracle.NodeIdxs[0]].ID)
	}
	if validated.TargetID != g.Nodes[oracle.NodeIdxs[len(oracle.NodeIdxs)-1]].ID {
		tb.Fatalf("route target %d does not match oracle target %d", validated.TargetID, g.Nodes[oracle.NodeIdxs[len(oracle.NodeIdxs)-1]].ID)
	}

	assertApprox32(tb, validated.Cost, oracle.Cost, "route cost mismatch against oracle")
	assertApprox32(tb, validated.Distance, oracle.Distance, "route distance mismatch against oracle")
}

func assertApprox32(tb testing.TB, actual, expected float32, message string) {
	tb.Helper()
	if float32(math.Abs(float64(actual-expected))) > FloatTolerance {
		tb.Fatalf("%s: got %.6f want %.6f", message, actual, expected)
	}
}
