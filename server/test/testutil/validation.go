package testutil

import (
	"math"
	"testing"

	"nav-system/src/graph/model"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
)

const FloatTolerance = float32(0.001)

type ValidatedRoute struct {
	EdgeIDs   []model.EdgeID
	Cost      float32
	Distance  float32
	SourceIdx uint32
	TargetIdx uint32
	FinalStep routingentities.Step
}

func AssertRouteValid(tb testing.TB, g *model.Graph, route routingentities.Route, wf routingengine.WeightFunc) ValidatedRoute {
	tb.Helper()

	if wf == nil {
		wf = routingengine.BaseWeight
	}
	if len(route.Steps) < 2 {
		tb.Fatalf("route must contain at least 2 steps, got %d", len(route.Steps))
	}
	if route.Steps[0].EdgeID != nil {
		tb.Fatalf("first step must not have an incoming edge")
	}

	var cumulativeDistance float32
	var cumulativeCost float32
	edgeIDs := make([]model.EdgeID, 0, len(route.Steps)-1)

	for i := 1; i < len(route.Steps); i++ {
		prev := route.Steps[i-1]
		cur := route.Steps[i]
		if cur.EdgeID == nil {
			tb.Fatalf("step %d missing edge id", i)
		}

		edgeID := model.EdgeID(*cur.EdgeID)
		if int(edgeID) >= len(g.Edges) {
			tb.Fatalf("step %d edge %d is out of range", i, edgeID)
		}
		edge := &g.Edges[edgeID]
		if edge.FromNode != prev.NodeIdx || edge.ToNode != cur.NodeIdx {
			tb.Fatalf("step %d edge %d does not connect %d -> %d", i, edgeID, prev.NodeIdx, cur.NodeIdx)
		}

		cumulativeDistance += edge.Length
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
		SourceIdx: route.Steps[0].NodeIdx,
		TargetIdx: route.Steps[len(route.Steps)-1].NodeIdx,
		FinalStep: route.Steps[len(route.Steps)-1],
	}
}

func AssertRouteMatchesOracle(tb testing.TB, g *model.Graph, route routingentities.Route, oracle OraclePath, wf routingengine.WeightFunc) {
	tb.Helper()

	validated := AssertRouteValid(tb, g, route, wf)
	if len(oracle.NodeIdxs) == 0 {
		tb.Fatalf("oracle path is empty")
	}

	if validated.SourceIdx != oracle.NodeIdxs[0] {
		tb.Fatalf("route source %d does not match oracle source %d", validated.SourceIdx, oracle.NodeIdxs[0])
	}
	if validated.TargetIdx != oracle.NodeIdxs[len(oracle.NodeIdxs)-1] {
		tb.Fatalf("route target %d does not match oracle target %d", validated.TargetIdx, oracle.NodeIdxs[len(oracle.NodeIdxs)-1])
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
