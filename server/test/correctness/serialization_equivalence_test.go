package correctness_test

import (
	"testing"

	"nav-system/src/graph/store"
	"nav-system/src/routing"
	"nav-system/test/testutil"
)

func TestSerializationRoundTripPreservesRoutingResults(t *testing.T) {
	corpora := []struct {
		graph string
		cases string
		size  int
	}{
		{graph: "diamond_graph.json", cases: "diamond_cases.json", size: 2},
		{graph: "one_way_detour_graph.json", cases: "one_way_detour_cases.json", size: 2},
		{graph: "disconnected_graph.json", cases: "disconnected_cases.json", size: 2},
	}

	for _, corpus := range corpora {
		corpus := corpus
		t.Run(corpus.graph, func(t *testing.T) {
			fixture := testutil.BuildGraphFixture(t, corpus.graph, corpus.size)
			routeCase := testutil.LoadRouteCases(t, corpus.cases)[0]

			before := fixture.Router.Compute(routeCase.Src.Lat, routeCase.Src.Lon, routeCase.Dst.Lat, routeCase.Dst.Lon, 1, routing.BaseWeight)
			if len(before) != 1 {
				t.Fatalf("expected 1 route before save, got %d", len(before))
			}

			dir := t.TempDir()
			if err := store.SaveGraph(fixture.Graph, dir); err != nil {
				t.Fatalf("SaveGraph: %v", err)
			}

			reloadedGraph, err := store.LoadGraph(dir)
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

			if beforeValidated.SourceIdx != afterValidated.SourceIdx || beforeValidated.TargetIdx != afterValidated.TargetIdx {
				t.Fatalf("route endpoints changed across save/load: before=%d->%d after=%d->%d", beforeValidated.SourceIdx, beforeValidated.TargetIdx, afterValidated.SourceIdx, afterValidated.TargetIdx)
			}
			if len(beforeValidated.EdgeIDs) != len(afterValidated.EdgeIDs) {
				t.Fatalf("edge count changed across save/load: before=%d after=%d", len(beforeValidated.EdgeIDs), len(afterValidated.EdgeIDs))
			}
			for i := range beforeValidated.EdgeIDs {
				if beforeValidated.EdgeIDs[i] != afterValidated.EdgeIDs[i] {
					t.Fatalf("edge %d changed across save/load: before=%d after=%d", i, beforeValidated.EdgeIDs[i], afterValidated.EdgeIDs[i])
				}
			}
		})
	}
}
