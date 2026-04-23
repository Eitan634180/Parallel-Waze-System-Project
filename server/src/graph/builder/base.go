package builder

import (
	"log"
	"math"
	"runtime"

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
	g.ProjectionRefLat = bbox.CenterLat()
	reprojectNodes(g)
	log.Printf("%s bounding box: lat [%.4f, %.4f] lon [%.4f, %.4f]",
		graphBuilderLogPrefix, bbox.MinLat, bbox.MaxLat, bbox.MinLon, bbox.MaxLon)
	log.Printf("%s building edges", graphBuilderLogPrefix)

	// Most ways contribute two directed edges per segment.
	estimatedEdges := len(pr.Ways) * graph.EstimatedEdgesPerWayHint
	g.Edges = make([]model.Edge, 0, estimatedEdges)

	// Build adjacency per node before converting it to CSR.
	adjTmp := make([][]model.EdgeID, len(g.Nodes))

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
		g.Base = buildCSR(adjTmp, len(g.Edges))
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
							FromNodeIdx: n1idx,
							ToNodeIdx:   n2idx,
							BaseWeight:  weightSec,
							DistanceM:   distM,
							SpeedKmh:    speedKmh,
							Flags:       flags,
						},
					})

					if !rw.IsOneWay {
						localEdges = append(localEdges, pendingEdge{
							fromIdx: n2idx,
							e: model.Edge{
								FromNodeIdx: n2idx,
								ToNodeIdx:   n1idx,
								BaseWeight:  weightSec,
								DistanceM:   distM,
								SpeedKmh:    speedKmh,
								Flags:       flags,
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
			fwdID := model.EdgeID(len(g.Edges))
			pe.e.ID = fwdID
			g.Edges = append(g.Edges, pe.e)
			adjTmp[pe.fromIdx] = append(adjTmp[pe.fromIdx], fwdID)
		}
	}

	log.Printf("%s edges ready (%d directed)", graphBuilderLogPrefix, len(g.Edges))
	log.Printf("%s building base adjacency", graphBuilderLogPrefix)
	g.Base = buildCSR(adjTmp, len(g.Edges))
	log.Printf("%s base adjacency ready", graphBuilderLogPrefix)

	return g, nodeIdx, nil
}

// buildCSR converts per-node edge slices into a CSR adjacency list.
func buildCSR(adjTmp [][]model.EdgeID, edgeCount int) model.BaseGraph {
	offsets := make([]uint32, len(adjTmp)+1)
	edgeIDs := make([]model.EdgeID, 0, edgeCount)

	for i, nbrs := range adjTmp {
		offsets[i] = uint32(len(edgeIDs))
		edgeIDs = append(edgeIDs, nbrs...)
	}
	offsets[len(adjTmp)] = uint32(len(edgeIDs))

	return model.BaseGraph{Offsets: offsets, EdgeIDs: edgeIDs}
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
		node.X, node.Y = utilities.ProjectAtReferenceLat(node.Lat, node.Lon, g.ProjectionRefLat)
	}
}
