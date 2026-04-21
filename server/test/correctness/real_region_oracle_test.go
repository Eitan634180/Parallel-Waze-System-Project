package correctness_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nav-system/src/graph/model"
	"nav-system/src/graph/store"
	"nav-system/src/routing"
	"nav-system/src/utilities"
	"nav-system/test/testutil"
)

type realRegionCorpusSpec struct {
	Region          string  `json:"region"`
	Seed            int64   `json:"seed"`
	Count           int     `json:"count"`
	MinNodeIndexGap int     `json:"min_node_index_gap"`
	MinDistanceM    float64 `json:"min_distance_m"`
	MaxDistanceM    float64 `json:"max_distance_m"`
	MaxAttempts     int     `json:"max_attempts"`
}

type realRegionQuery struct {
	Name   string
	SrcIdx uint32
	DstIdx uint32
}

func TestRealRegionCrossCellRoutesMatchOracle(t *testing.T) {
	corpora := []string{
		"Sweden-bench.json",
		"israel-and-palestine-bench.json",
	}

	for _, corpusName := range corpora {
		corpusName := corpusName
		t.Run(corpusName, func(t *testing.T) {
			spec := loadRealRegionCorpusSpec(t, corpusName)
			graph := loadRealRegionGraph(t, spec.Region)
			snap := routing.BuildSnapIndex(graph)
			router := routing.NewRouterWithMode(graph, snap, routing.RoutingModeHierarchical)
			queries := buildRealRegionCrossCellCorpus(t, graph, spec)
			if len(queries) == 0 {
				t.Fatalf("no cross-cell queries generated for region %q", spec.Region)
			}

			for _, query := range queries {
				query := query
				t.Run(query.Name, func(t *testing.T) {
					oracle, ok := testutil.ShortestPath(graph, query.SrcIdx, query.DstIdx, routing.BaseWeight)
					if !ok {
						t.Fatalf("oracle found no route for %s", query.Name)
					}

					routes := router.ComputeFromIndices(query.SrcIdx, query.DstIdx, 1, routing.BaseWeight)
					if len(routes) != 1 {
						t.Fatalf("expected 1 hierarchical route, got %d", len(routes))
					}

					assertRealRegionRouteMatchesOracleCost(t, graph, routes[0], oracle)
				})
			}
		})
	}
}

func assertRealRegionRouteMatchesOracleCost(t *testing.T, g *model.Graph, route routing.Route, oracle testutil.OraclePath) {
	t.Helper()

	validated := testutil.AssertRouteValid(t, g, route, routing.BaseWeight)
	if len(oracle.NodeIdxs) == 0 {
		t.Fatal("oracle path is empty")
	}
	if validated.SourceIdx != oracle.NodeIdxs[0] {
		t.Fatalf("route source %d does not match oracle source %d", validated.SourceIdx, oracle.NodeIdxs[0])
	}
	if validated.TargetIdx != oracle.NodeIdxs[len(oracle.NodeIdxs)-1] {
		t.Fatalf("route target %d does not match oracle target %d", validated.TargetIdx, oracle.NodeIdxs[len(oracle.NodeIdxs)-1])
	}
	assertApprox32Local(t, validated.Cost, oracle.Cost, "route cost mismatch against oracle")
}

func assertApprox32Local(t *testing.T, actual, expected float32, message string) {
	t.Helper()
	if float32(math.Abs(float64(actual-expected))) > testutil.FloatTolerance {
		t.Fatalf("%s: got %.6f want %.6f", message, actual, expected)
	}
}

func buildRealRegionCrossCellCorpus(tb testing.TB, g *model.Graph, spec realRegionCorpusSpec) []realRegionQuery {
	tb.Helper()

	if spec.Count <= 0 {
		tb.Fatalf("invalid corpus count %d", spec.Count)
	}
	if len(g.Nodes) == 0 {
		tb.Fatal("graph contains no nodes")
	}

	maxAttempts := spec.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = spec.Count * 200
	}

	rng := rand.New(rand.NewSource(spec.Seed))
	baseAStar := routing.NewRouterWithMode(g, nil, routing.RoutingModeBaseAStar)
	seen := make(map[[2]uint32]struct{}, spec.Count)
	queries := make([]realRegionQuery, 0, spec.Count)

	for attempts := 0; len(queries) < spec.Count && attempts < maxAttempts; attempts++ {
		srcIdx := uint32(rng.Intn(len(g.Nodes)))
		dstIdx := uint32(rng.Intn(len(g.Nodes)))
		if srcIdx == dstIdx {
			continue
		}
		if g.Nodes[srcIdx].CellID == g.Nodes[dstIdx].CellID {
			continue
		}

		if spec.MinNodeIndexGap > 0 {
			gap := int(srcIdx) - int(dstIdx)
			if gap < 0 {
				gap = -gap
			}
			if gap < spec.MinNodeIndexGap {
				continue
			}
		}

		src := g.Nodes[srcIdx]
		dst := g.Nodes[dstIdx]
		dist := utilities.HaversineM(src.Lat, src.Lon, dst.Lat, dst.Lon)
		if spec.MinDistanceM > 0 {
			if dist < spec.MinDistanceM || dist > spec.MaxDistanceM {
				continue
			}
		}

		key := [2]uint32{srcIdx, dstIdx}
		if _, ok := seen[key]; ok {
			continue
		}

		routes := baseAStar.ComputeFromIndices(srcIdx, dstIdx, 1, routing.BaseWeight)
		if len(routes) == 0 {
			continue
		}

		queries = append(queries, realRegionQuery{
			Name:   fmt.Sprintf("case-%03d", len(queries)+1),
			SrcIdx: srcIdx,
			DstIdx: dstIdx,
		})
		seen[key] = struct{}{}
	}

	if len(queries) != spec.Count {
		tb.Fatalf("generated %d/%d cross-cell queries for region %q", len(queries), spec.Count, spec.Region)
	}
	return queries
}

func loadRealRegionCorpusSpec(tb testing.TB, corpusName string) realRegionCorpusSpec {
	tb.Helper()

	path := filepath.Join(realRegionTestdataRoot(), "benchmark-cases", corpusName)
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read corpus spec %s: %v", path, err)
	}

	var spec realRegionCorpusSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		tb.Fatalf("decode corpus spec %s: %v", path, err)
	}
	return spec
}

func loadRealRegionGraph(tb testing.TB, region string) *model.Graph {
	tb.Helper()

	graph, err := store.LoadGraph(filepath.Join(realRegionModuleRoot(), "data", "map", region))
	if err != nil {
		tb.Fatalf("LoadGraph(%s): %v", region, err)
	}
	return graph
}

func realRegionTestdataRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata")
}

func realRegionModuleRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}
