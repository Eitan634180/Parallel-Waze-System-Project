package correctness_test

import (
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/test/testutil"
)

func TestSerializationRoundTripPreservesRoutingResults(t *testing.T) {
	fixture := testutil.BuildGraphFixture(t, "diamond_graph.json", 2)
	routeCase := testutil.LoadRouteCases(t, "diamond_cases.json")[0]

	before := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routing.BaseWeight)
	if len(before) != 1 {
		t.Fatalf("expected 1 route before save, got %d", len(before))
	}

	dir := t.TempDir()
	if err := builder.SaveGraph(fixture.Graph, dir); err != nil {
		t.Fatalf("SaveGraph: %v", err)
	}

	reloadedGraph, err := builder.LoadGraph(dir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}
	reloadedRouter := routing.NewRouter(reloadedGraph, routing.BuildSnapIndex(reloadedGraph))

	after := reloadedRouter.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routing.BaseWeight)
	if len(after) != 1 {
		t.Fatalf("expected 1 route after load, got %d", len(after))
	}

	beforeValidated := testutil.AssertRouteValid(t, fixture.Graph, before[0], routing.BaseWeight)
	afterValidated := testutil.AssertRouteValid(t, reloadedGraph, after[0], routing.BaseWeight)

	if beforeValidated.SourceID != afterValidated.SourceID || beforeValidated.TargetID != afterValidated.TargetID {
		t.Fatalf("route endpoints changed across save/load: before=%d->%d after=%d->%d", beforeValidated.SourceID, beforeValidated.TargetID, afterValidated.SourceID, afterValidated.TargetID)
	}
	if len(beforeValidated.EdgeIDs) != len(afterValidated.EdgeIDs) {
		t.Fatalf("edge count changed across save/load: before=%d after=%d", len(beforeValidated.EdgeIDs), len(afterValidated.EdgeIDs))
	}
	for i := range beforeValidated.EdgeIDs {
		if beforeValidated.EdgeIDs[i] != afterValidated.EdgeIDs[i] {
			t.Fatalf("edge %d changed across save/load: before=%d after=%d", i, beforeValidated.EdgeIDs[i], afterValidated.EdgeIDs[i])
		}
	}
}
