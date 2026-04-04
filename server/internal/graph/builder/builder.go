package builder

import (
	"fmt"
	"sort"
)

// BuildGraph constructs the base graph from parsed OSM nodes and ways.
func BuildGraph(pr *ParseResult) (*Graph, error) {
	g := &Graph{
		NodeIdx:         make(map[NodeID]uint32, len(pr.Nodes)),
		CellIdx:         make(map[NodeID]CellID),
		BoundaryNodeIdx: make(map[NodeID]uint32),
	}

	fmt.Println("[BUILDER] Building nodes ...")

	g.Nodes = make([]Node, 0, len(pr.Nodes))
	for _, rn := range pr.Nodes {
		idx := uint32(len(g.Nodes))
		g.NodeIdx[rn.ID] = idx
		x, y := ProjectXY(rn.Lat, rn.Lon)
		g.Nodes = append(g.Nodes, Node{
			ID:  rn.ID,
			Lat: rn.Lat,
			Lon: rn.Lon,
			X:   x,
			Y:   y,
		})
	}

	fmt.Printf("[BUILDER] %d nodes built\n", len(g.Nodes))
	fmt.Println("[BUILDER] Building edges ...")

	// Most ways contribute two directed edges per segment.
	estimatedEdges := len(pr.Ways) * 4
	g.Edges = make([]Edge, 0, estimatedEdges)

	// Build adjacency per node before converting it to CSR.
	adjTmp := make([][]EdgeID, len(g.Nodes))

	for _, rw := range pr.Ways {
		speedKmh := rw.MaxSpeed
		if speedKmh == 0 {
			speedKmh = DefaultSpeedKmh(rw.RoadClass)
		}

		for i := 0; i < len(rw.NodeRefs)-1; i++ {
			n1id := rw.NodeRefs[i]
			n2id := rw.NodeRefs[i+1]

			n1idx, ok1 := g.NodeIdx[n1id]
			n2idx, ok2 := g.NodeIdx[n2id]
			if !ok1 || !ok2 {
				continue
			}

			n1 := &g.Nodes[n1idx]
			n2 := &g.Nodes[n2idx]

			distM := float32(HaversineM(n1.Lat, n1.Lon, n2.Lat, n2.Lon))
			weightSec := distM / (speedKmh / 3.6)

			flags := uint8(0)
			if rw.IsOneWay {
				flags |= FlagOneWay
			}

			fwdID := EdgeID(len(g.Edges))
			g.Edges = append(g.Edges, Edge{
				ID:         fwdID,
				FromNodeID: n1id,
				ToNodeID:   n2id,
				Weight:     weightSec,
				DistanceM:  distM,
				SpeedKmh:   speedKmh,
				RoadClass:  rw.RoadClass,
				Flags:      flags,
			})
			adjTmp[n1idx] = append(adjTmp[n1idx], fwdID)

			if !rw.IsOneWay {
				revID := EdgeID(len(g.Edges))
				g.Edges = append(g.Edges, Edge{
					ID:         revID,
					FromNodeID: n2id,
					ToNodeID:   n1id,
					Weight:     weightSec,
					DistanceM:  distM,
					SpeedKmh:   speedKmh,
					RoadClass:  rw.RoadClass,
					Flags:      flags,
				})
				adjTmp[n2idx] = append(adjTmp[n2idx], revID)
			}
		}
	}

	fmt.Printf("[BUILDER] %d directed edges built\n", len(g.Edges))
	fmt.Println("[BUILDER] Building base CSR adjacency list ...")
	g.BaseAdj = buildCSR(adjTmp, len(g.Edges))
	fmt.Println("[BUILDER] Base CSR done")

	return g, nil
}

// buildCSR converts per-node edge slices into a CSR adjacency list.
func buildCSR(adjTmp [][]EdgeID, edgeCount int) AdjacencyList {
	offsets := make([]uint32, len(adjTmp)+1)
	edgeIDs := make([]EdgeID, 0, edgeCount)

	for i, nbrs := range adjTmp {
		offsets[i] = uint32(len(edgeIDs))
		edgeIDs = append(edgeIDs, nbrs...)
	}
	offsets[len(adjTmp)] = uint32(len(edgeIDs))

	return AdjacencyList{Offsets: offsets, EdgeIDs: edgeIDs}
}

// SortedEdgesFrom returns all edge IDs leaving nodeID, sorted by ToNodeID.
func (g *Graph) SortedEdgesFrom(nodeID NodeID) []EdgeID {
	idx, ok := g.NodeIdx[nodeID]
	if !ok {
		return nil
	}
	raw := g.BaseAdj.Neighbours(idx)
	out := make([]EdgeID, len(raw))
	copy(out, raw)
	sort.Slice(out, func(i, j int) bool {
		return g.Edges[out[i]].ToNodeID < g.Edges[out[j]].ToNodeID
	})
	return out
}
