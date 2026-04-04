package builder

import "sync"

// ---------------------------------------------------------------------------
// ID type aliases
// ---------------------------------------------------------------------------

// NodeID is an OSM node ID (64-bit, matches OSM spec).
type NodeID = uint64

// EdgeID indexes into the Graph.Edges slice (32-bit is enough for ~4B edges).
type EdgeID = uint32

// CellID indexes into the Graph.Cells slice.
type CellID = uint32

// ---------------------------------------------------------------------------
// Road classification
// ---------------------------------------------------------------------------

const (
	RoadMotorway     uint8 = 1
	RoadTrunk        uint8 = 2
	RoadPrimary      uint8 = 3
	RoadSecondary    uint8 = 4
	RoadTertiary     uint8 = 5
	RoadResidential  uint8 = 6
	RoadService      uint8 = 7
	RoadUnclassified uint8 = 8
)

// DefaultSpeedKmh returns the assumed speed for a road class when no maxspeed tag is present.
func DefaultSpeedKmh(class uint8) float32 {
	switch class {
	case RoadMotorway:
		return 120
	case RoadTrunk:
		return 100
	case RoadPrimary:
		return 80
	case RoadSecondary:
		return 60
	case RoadTertiary:
		return 50
	case RoadResidential:
		return 50
	case RoadService:
		return 20
	default:
		return 50
	}
}

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// Node represents a road-network vertex (OSM node on a highway way).
type Node struct {
	ID     NodeID
	Lat    float64 // geographic latitude  (WGS-84)
	Lon    float64 // geographic longitude (WGS-84)
	X      float32 // equirectangular projected X (meters) — for A* heuristic
	Y      float32 // equirectangular projected Y (meters) — for A* heuristic
	CellID CellID  // which partition cell this node belongs to
}

// ---------------------------------------------------------------------------
// Edge (base graph, directed)
// ---------------------------------------------------------------------------

// Edge flags (bit positions).
const (
	FlagOneWay uint8 = 1 << 0
	FlagToll   uint8 = 1 << 1
)

// Edge is a directed edge in the base road graph.
type Edge struct {
	ID         EdgeID
	FromNodeID NodeID
	ToNodeID   NodeID
	Weight     float32 // primary cost: travel time in seconds
	DistanceM  float32 // physical length in metres
	SpeedKmh   float32 // speed used to compute Weight
	RoadClass  uint8
	Flags      uint8 // bitmask of FlagOneWay, FlagToll, …
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }

// ---------------------------------------------------------------------------
// Overlay edge
// ---------------------------------------------------------------------------

// OverlayEdge is an edge in the overlay (two-level) graph.
//
// Two kinds exist:
//
//	Cross-cell edge  (IsCrossCell == true):
//	  Copied directly from the base graph. Connects a boundary node in one cell
//	  to a boundary node in a neighbouring cell. These are the partition cut edges.
//
//	Intra-cell shortcut  (IsCrossCell == false):
//	  Precomputed via intra-cell Dijkstra. Connects two boundary nodes in the SAME
//	  cell. Weight = shortest-path cost through cell interior.
//	  The full node sequence is NOT stored — it is re-derived lazily via a small
//	  intra-cell Dijkstra during route reconstruction.
type OverlayEdge struct {
	FromNodeID  NodeID
	ToNodeID    NodeID
	Weight      float32 // travel time (seconds)
	DistanceM   float32 // shortest-path distance (metres)
	IsCrossCell bool
}

// ---------------------------------------------------------------------------
// CSR adjacency list
// ---------------------------------------------------------------------------

// AdjacencyList stores directed adjacency in Compressed Sparse Row format for
// cache-friendly iteration.
//
// For node with internal index i:
//
//	outgoing edge IDs = EdgeIDs[ Offsets[i] : Offsets[i+1] ]
type AdjacencyList struct {
	Offsets []uint32 // length = num_nodes + 1
	EdgeIDs []EdgeID // flat list of outgoing edge IDs, length = num_edges
}

// Neighbours returns the slice of EdgeIDs leaving node at internal index idx.
func (a *AdjacencyList) Neighbours(idx uint32) []EdgeID {
	return a.EdgeIDs[a.Offsets[idx]:a.Offsets[idx+1]]
}

// ---------------------------------------------------------------------------
// Cell
// ---------------------------------------------------------------------------

// Cell is one partition region produced by Inertial Flow recursive bisection.
type Cell struct {
	ID              CellID
	InternalNodeIDs []NodeID // ALL nodes assigned to this cell (includes boundary)
	BoundaryNodeIDs []NodeID // nodes that have at least one cross-cell edge
}

// ---------------------------------------------------------------------------
// Overlay adjacency list
// ---------------------------------------------------------------------------

// OverlayAdjList is a CSR adjacency list for overlay edges only.
// The "index" space is the set of boundary nodes; use OverlayNodeIndex to map
// NodeID → index.
type OverlayAdjList struct {
	Mu           sync.RWMutex
	Offsets      []uint32      // length = num_boundary_nodes + 1
	OverlayEdges []OverlayEdge // flat list indexed by Offsets
}

// Neighbours returns overlay edges leaving boundary node at internal overlay index idx.
func (o *OverlayAdjList) Neighbours(idx uint32) []OverlayEdge {
	return o.OverlayEdges[o.Offsets[idx]:o.Offsets[idx+1]]
}

// ---------------------------------------------------------------------------
// Graph — top-level container
// ---------------------------------------------------------------------------

// Graph holds all data structures for the road network.
type Graph struct {
	// Base graph
	Nodes   []Node            // indexed by internal node index (0-based)
	Edges   []Edge            // indexed by EdgeID
	NodeIdx map[NodeID]uint32 // OSM NodeID → internal array index
	BaseAdj AdjacencyList     // base-graph adjacency (index = internal node index)

	// Partition
	Cells           []Cell
	CellIdx         map[NodeID]CellID // NodeID → CellID (boundary nodes appear here too)
	BoundaryNodeIdx map[NodeID]uint32 // NodeID → index in the overlay node list
	BoundaryNodes   []NodeID          // ordered list of all boundary nodes

	// Overlay graph
	OverlayAdj OverlayAdjList
}

// NodeByID returns a pointer to the Node for a given OSM NodeID.
func (g *Graph) NodeByID(id NodeID) *Node {
	idx, ok := g.NodeIdx[id]
	if !ok {
		return nil
	}
	return &g.Nodes[idx]
}
