package builder

import (
	"log"
	"math"
	"runtime"
	"slices"

	"nav-system/src/graph"
	"nav-system/src/graph/model"
	"nav-system/src/utilities"

	"golang.org/x/sync/errgroup"
)

const graphBuilderLogPrefix = "graph-builder:"

// BuildBaseGraph constructs the base graph from parsed OSM nodes and ways.
func BuildBaseGraph(pr *ParseResult) (*model.Graph, map[NodeRawID]uint32, error) {
	g := &model.Graph{}
	nodeIdx := make(map[NodeRawID]uint32, len(pr.Nodes))

	log.Printf("%s building nodes", graphBuilderLogPrefix)

	g.Nodes = make([]model.Node, 0, len(pr.Nodes))
	for _, rn := range pr.Nodes {
		idx := uint32(len(g.Nodes))
		nodeIdx[rn.ID] = idx
		g.Nodes = append(g.Nodes, model.Node{
			Lat: rn.Lat,
			Lon: rn.Lon,
		})
	}

	log.Printf("%s nodes ready (%d)", graphBuilderLogPrefix, len(g.Nodes))

	// Compute bounding box from all nodes.
	bbox := boundingBoxFromNodes(g.Nodes)
	g.BBox = bbox
	g.RefLat = bbox.CenterLat()
	reprojectNodes(g)
	log.Printf("%s bounding box: lat [%.4f, %.4f] lon [%.4f, %.4f]",
		graphBuilderLogPrefix, bbox.MinLat, bbox.MaxLat, bbox.MinLon, bbox.MaxLon)
	log.Printf("%s building edges", graphBuilderLogPrefix)

	// Most ways contribute two directed edges per segment.
	estimatedEdges := len(pr.Ways) * graph.EstimatedEdgesPerWayHint
	g.Edges = make([]model.Edge, 0, estimatedEdges)

	type pendingEdge struct {
		fromIdx uint32
		e       model.Edge
	}

	workerCount := max(runtime.GOMAXPROCS(0), 1)
	if workerCount < graph.MinGraphBuilderWorkers {
		workerCount = graph.MinGraphBuilderWorkers
	}
	if len(pr.Ways) == 0 {
		log.Printf("%s edges ready (%d directed)", graphBuilderLogPrefix, len(g.Edges))
		log.Printf("%s building base adjacency", graphBuilderLogPrefix)
		g.Base = buildCSR(nil, len(g.Nodes))
		log.Printf("%s base adjacency ready", graphBuilderLogPrefix)
		return g, nodeIdx, nil
	}
	if workerCount > len(pr.Ways) {
		workerCount = len(pr.Ways)
	}

	chunkSize := (len(pr.Ways) + workerCount - 1) / workerCount
	var eg errgroup.Group
	workerResults := make([][]pendingEdge, workerCount)

	for w := 0; w < workerCount; w++ {
		w := w
		eg.Go(func() error {
			start := w * chunkSize
			if start >= len(pr.Ways) {
				return nil
			}

			end := start + chunkSize
			if end > len(pr.Ways) {
				end = len(pr.Ways)
			}

			localEdges := make([]pendingEdge, 0, (end-start)*graph.PendingEdgesPerSegmentHint)

			for i := start; i < end; i++ {
				rw := pr.Ways[i]
				speedKmh := rw.MaxSpeed
				if speedKmh == 0 {
					speedKmh = model.DefaultSpeedKmh(rw.RoadClass)
				}

				for j := 0; j < len(rw.NodeRefs)-1; j++ {
					n1id := rw.NodeRefs[j]
					n2id := rw.NodeRefs[j+1]

					n1idx, ok1 := nodeIdx[n1id]
					n2idx, ok2 := nodeIdx[n2id]
					if !ok1 || !ok2 {
						continue
					}

					n1 := &g.Nodes[n1idx]
					n2 := &g.Nodes[n2idx]

					distM := float32(utilities.HaversineM(n1.Lat, n1.Lon, n2.Lat, n2.Lon))
					weightSec := distM / (speedKmh / utilities.KilometersPerHourToMps)

					flags := uint8(0)
					if rw.IsOneWay {
						flags |= model.FlagOneWay
					}

					localEdges = append(localEdges, pendingEdge{
						fromIdx: n1idx,
						e: model.Edge{
							SrcNode:    n1idx,
							DstNode:    n2idx,
							BaseWeight: weightSec,
							Length:     distM,
							SpeedLimit: speedKmh,
							Flags:      flags,
						},
					})

					if !rw.IsOneWay {
						localEdges = append(localEdges, pendingEdge{
							fromIdx: n2idx,
							e: model.Edge{
								SrcNode:    n2idx,
								DstNode:    n1idx,
								BaseWeight: weightSec,
								Length:     distM,
								SpeedLimit: speedKmh,
								Flags:      flags,
							},
						})
					}
				}
			}
			workerResults[w] = localEdges
			return nil
		})
	}
	_ = eg.Wait()

	totalEdges := 0
	for _, res := range workerResults {
		totalEdges += len(res)
	}
	g.Edges = make([]model.Edge, 0, totalEdges)
	for _, res := range workerResults {
		for _, pe := range res {
			g.Edges = append(g.Edges, pe.e)
		}
	}

	slices.SortFunc(g.Edges, func(a, b model.Edge) int {
		if a.SrcNode < b.SrcNode {
			return -1
		}
		if a.SrcNode > b.SrcNode {
			return 1
		}
		if a.DstNode < b.DstNode {
			return -1
		}
		if a.DstNode > b.DstNode {
			return 1
		}
		return 0
	})

	for i := range g.Edges {
		g.Edges[i].ID = model.EdgeID(i)
	}

	log.Printf("%s edges ready (%d directed)", graphBuilderLogPrefix, len(g.Edges))
	log.Printf("%s building base adjacency", graphBuilderLogPrefix)
	g.Base = buildCSR(g.Edges, len(g.Nodes))
	log.Printf("%s base adjacency ready", graphBuilderLogPrefix)

	return g, nodeIdx, nil
}

func buildCSR(edges []model.Edge, nodeCount int) model.BaseGraph {
	offsets := make([]uint32, nodeCount+1)
	for _, e := range edges {
		offsets[e.SrcNode+1]++
	}
	for i := 1; i <= nodeCount; i++ {
		offsets[i] += offsets[i-1]
	}
	return model.BaseGraph{Offsets: offsets}
}

// ReorderGraphByCell sorts g.Nodes by CellID, updates edge references, and then
// sorts g.Edges by FromNode. This perfectly groups both nodes and edges by cell in memory.
// It also rebuilds the offsets-only BaseGraph CSR.
func ReorderGraphByCell(g *model.Graph, nodeIdx map[NodeRawID]uint32) {
	log.Printf("%s reordering graph by cell for cache locality", graphBuilderLogPrefix)

	type sortedNode struct {
		n       model.Node
		origIdx uint32
	}
	sn := make([]sortedNode, len(g.Nodes))
	for i := range g.Nodes {
		sn[i] = sortedNode{n: g.Nodes[i], origIdx: uint32(i)}
	}
	slices.SortFunc(sn, func(a, b sortedNode) int {
		if a.n.CellID < b.n.CellID {
			return -1
		}
		if a.n.CellID > b.n.CellID {
			return 1
		}
		if a.origIdx < b.origIdx {
			return -1
		}
		if a.origIdx > b.origIdx {
			return 1
		}
		return 0
	})

	oldToNew := make([]uint32, len(g.Nodes))
	for i := range sn {
		g.Nodes[i] = sn[i].n
		oldToNew[sn[i].origIdx] = uint32(i)
	}

	if nodeIdx != nil {
		for rawID, oldIdx := range nodeIdx {
			nodeIdx[rawID] = oldToNew[oldIdx]
		}
	}

	for i := range g.Edges {
		g.Edges[i].SrcNode = oldToNew[g.Edges[i].SrcNode]
		g.Edges[i].DstNode = oldToNew[g.Edges[i].DstNode]
	}

	slices.SortFunc(g.Edges, func(a, b model.Edge) int {
		if a.SrcNode < b.SrcNode {
			return -1
		}
		if a.SrcNode > b.SrcNode {
			return 1
		}
		if a.DstNode < b.DstNode {
			return -1
		}
		if a.DstNode > b.DstNode {
			return 1
		}
		return 0
	})

	for i := range g.Edges {
		g.Edges[i].ID = model.EdgeID(i)
	}

	g.Base = buildCSR(g.Edges, len(g.Nodes))
}

func boundingBoxFromNodes(nodes []model.Node) model.BoundingBox {
	if len(nodes) == 0 {
		return model.BoundingBox{}
	}

	bbox := model.BoundingBox{
		MinLat: math.Inf(1),
		MaxLat: math.Inf(-1),
		MinLon: math.Inf(1),
		MaxLon: math.Inf(-1),
	}
	for i := range nodes {
		node := &nodes[i]
		if node.Lat < bbox.MinLat {
			bbox.MinLat = node.Lat
		}
		if node.Lat > bbox.MaxLat {
			bbox.MaxLat = node.Lat
		}
		if node.Lon < bbox.MinLon {
			bbox.MinLon = node.Lon
		}
		if node.Lon > bbox.MaxLon {
			bbox.MaxLon = node.Lon
		}
	}
	return bbox
}

func reprojectNodes(g *model.Graph) {
	for i := range g.Nodes {
		node := &g.Nodes[i]
		node.X, node.Y = utilities.ProjectAtReferenceLat(node.Lat, node.Lon, g.RefLat)
	}
}
