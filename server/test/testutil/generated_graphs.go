package testutil

import (
	"fmt"
	"math/rand"
	"testing"

	"nav-system/src/graph/builder"
	"nav-system/src/graph/model"
	routingengine "nav-system/src/routing/engine"
)

type GeneratedQuery struct {
	Name      string
	SrcNodeID model.NodeRawID
	DstNodeID model.NodeRawID
}

type GeneratedWeightedFixture struct {
	Graph      *model.Graph
	Snap       *routingengine.SnapIndex
	Router     *routingengine.Router
	WeightFunc routingengine.WeightFunc
	NodeIDs    []model.NodeRawID
	Queries    []GeneratedQuery
}

type generatedCorridorSpec struct {
	seed        int64
	width       int
	height      int
	nodeSpacing float64
	queryCount  int
	maxCellSize int
	allowOneWay bool
}

func BuildGeneratedWeightedFixture(tb testing.TB, seed int64, width, height, queryCount, maxCellSize int) *GeneratedWeightedFixture {
	tb.Helper()

	spec := generatedCorridorSpec{
		seed:        seed,
		width:       width,
		height:      height,
		nodeSpacing: 0.001,
		queryCount:  queryCount,
		maxCellSize: maxCellSize,
		allowOneWay: false,
	}
	return buildGeneratedCorridorFixture(tb, spec)
}

func BuildGeneratedDirectedFixture(tb testing.TB, seed int64, width, height, queryCount, maxCellSize int) *GeneratedWeightedFixture {
	tb.Helper()

	spec := generatedCorridorSpec{
		seed:        seed,
		width:       width,
		height:      height,
		nodeSpacing: 0.001,
		queryCount:  queryCount,
		maxCellSize: maxCellSize,
		allowOneWay: true,
	}
	return buildGeneratedCorridorFixture(tb, spec)
}

func buildGeneratedCorridorFixture(tb testing.TB, spec generatedCorridorSpec) *GeneratedWeightedFixture {
	tb.Helper()

	if spec.width < 2 || spec.height < 2 {
		tb.Fatalf("generated graph dimensions must be at least 2x2, got %dx%d", spec.width, spec.height)
	}
	if spec.queryCount <= 0 {
		tb.Fatalf("generated graph queryCount must be positive, got %d", spec.queryCount)
	}

	rng := rand.New(rand.NewSource(spec.seed))
	parseResult, nodeIDs := generatedGridParseResult(spec, rng)
	g, err := builder.BuildBaseGraph(parseResult)
	if err != nil {
		tb.Fatalf("BuildGraph(seed=%d): %v", spec.seed, err)
	}

	builder.PartitionCells(g, spec.maxCellSize)
	builder.DetectBoundaryNodes(g)
	builder.BuildOverlayGraph(g, 1)

	snap := routingengine.BuildSnapIndex(g)
	queries := generatedQueries(spec.seed, nodeIDs, spec.queryCount, rng)
	multipliers := generatedMultipliers(g, rng)
	wf := generatedWeightFunc(multipliers)

	return &GeneratedWeightedFixture{
		Graph:      g,
		Snap:       snap,
		Router:     routingengine.NewRouter(g, snap),
		WeightFunc: wf,
		NodeIDs:    nodeIDs,
		Queries:    queries,
	}
}

func generatedGridParseResult(spec generatedCorridorSpec, rng *rand.Rand) (*builder.ParseResult, []model.NodeRawID) {
	nodes := make(map[uint64]*builder.RawNode, spec.width*spec.height)
	nodeIDs := make([]model.NodeRawID, 0, spec.width*spec.height)

	const (
		baseLat = 32.0
		baseLon = 34.0
	)

	nodeID := uint64(1)
	indexAt := make([][]uint64, spec.height)
	for row := 0; row < spec.height; row++ {
		indexAt[row] = make([]uint64, spec.width)
		for col := 0; col < spec.width; col++ {
			id := nodeID
			nodeID++
			indexAt[row][col] = id
			nodes[id] = &builder.RawNode{
				ID:  id,
				Lat: baseLat + float64(row)*spec.nodeSpacing,
				Lon: baseLon + float64(col)*spec.nodeSpacing,
			}
			nodeIDs = append(nodeIDs, id)
		}
	}

	ways := make([]*builder.RawWay, 0, spec.width*spec.height*2)
	wayID := uint64(10_000 + spec.seed*100)

	for row := 0; row < spec.height; row++ {
		for col := 0; col < spec.width-1; col++ {
			from := indexAt[row][col]
			to := indexAt[row][col+1]
			oneWay := spec.allowOneWay && rng.Intn(4) == 0
			refs := []uint64{from, to}
			if oneWay && rng.Intn(2) == 0 {
				refs = []uint64{to, from}
			}
			ways = append(ways, &builder.RawWay{
				ID:        wayID,
				NodeRefs:  refs,
				RoadClass: horizontalRoadClass(row, col),
				IsOneWay:  oneWay,
				MaxSpeed:  horizontalSpeedKmh(row, col),
			})
			wayID++
		}
	}

	for row := 0; row < spec.height-1; row++ {
		for col := 0; col < spec.width; col++ {
			from := indexAt[row][col]
			to := indexAt[row+1][col]
			oneWay := spec.allowOneWay && rng.Intn(5) == 0
			refs := []uint64{from, to}
			if oneWay && rng.Intn(2) == 0 {
				refs = []uint64{to, from}
			}
			ways = append(ways, &builder.RawWay{
				ID:        wayID,
				NodeRefs:  refs,
				RoadClass: verticalRoadClass(row, col),
				IsOneWay:  oneWay,
				MaxSpeed:  verticalSpeedKmh(row, col),
			})
			wayID++
		}
	}

	return &builder.ParseResult{
		Nodes: nodes,
		Ways:  ways,
	}, nodeIDs
}

func generatedQueries(seed int64, nodeIDs []model.NodeRawID, queryCount int, rng *rand.Rand) []GeneratedQuery {
	queries := make([]GeneratedQuery, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		srcNode := nodeIDs[rng.Intn(len(nodeIDs))]
		dstNode := nodeIDs[rng.Intn(len(nodeIDs))]
		if len(nodeIDs) > 1 {
			for dstNode == srcNode {
				dstNode = nodeIDs[rng.Intn(len(nodeIDs))]
			}
		}
		queries = append(queries, GeneratedQuery{
			Name:      fmt.Sprintf("seed_%d_query_%02d", seed, i),
			SrcNodeID: srcNode,
			DstNodeID: dstNode,
		})
	}
	return queries
}

func ExhaustiveGeneratedQueries(nodeIDs []model.NodeRawID) []GeneratedQuery {
	queries := make([]GeneratedQuery, 0, len(nodeIDs)*(len(nodeIDs)-1))
	for _, srcNodeID := range nodeIDs {
		for _, dstNodeID := range nodeIDs {
			if srcNodeID == dstNodeID {
				continue
			}
			queries = append(queries, GeneratedQuery{
				Name:      fmt.Sprintf("%d_to_%d", srcNodeID, dstNodeID),
				SrcNodeID: srcNodeID,
				DstNodeID: dstNodeID,
			})
		}
	}
	return queries
}

func generatedMultipliers(g *model.Graph, rng *rand.Rand) []float32 {
	multipliers := make([]float32, len(g.Edges))
	for i := range multipliers {
		bucket := i % 5
		base := float32(0.85 + float32(bucket)*0.2)
		jitter := float32(rng.Intn(5)) * 0.03
		multipliers[i] = base + jitter
	}
	return multipliers
}

func generatedWeightFunc(multipliers []float32) routingengine.WeightFunc {
	return func(edge *model.Edge) float32 {
		weight := edge.BaseWeight
		if int(edge.ID) < len(multipliers) {
			weight *= multipliers[int(edge.ID)]
		}
		return weight
	}
}

func horizontalRoadClass(row, col int) uint8 {
	if (row+col)%3 == 0 {
		return model.RoadPrimary
	}
	return model.RoadSecondary
}

func verticalRoadClass(row, col int) uint8 {
	if (row+col)%2 == 0 {
		return model.RoadResidential
	}
	return model.RoadTertiary
}

func horizontalSpeedKmh(row, col int) float32 {
	return float32(35 + ((row+col)%4)*10)
}

func verticalSpeedKmh(row, col int) float32 {
	return float32(30 + ((row*2+col)%4)*8)
}
