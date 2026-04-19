package builder

import (
	"fmt"
	"sync"
)

// NodeID is an OSM node ID.
type NodeID = uint64

// EdgeID indexes into Graph.Edges.
type EdgeID = uint32

// CellID indexes into Graph.Cells.
type CellID = uint32

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

const (
	defaultMotorwaySpeedKmh     = float32(120)
	defaultTrunkSpeedKmh        = float32(100)
	defaultPrimarySpeedKmh      = float32(80)
	defaultSecondarySpeedKmh    = float32(60)
	defaultTertiarySpeedKmh     = float32(50)
	defaultResidentialSpeedKmh  = float32(50)
	defaultServiceSpeedKmh      = float32(20)
	defaultUnclassifiedSpeedKmh = float32(50)
)

// DefaultSpeedKmh returns the fallback speed for a road class when maxspeed is
// missing from the source data.
func DefaultSpeedKmh(class uint8) float32 {
	switch class {
	case RoadMotorway:
		return defaultMotorwaySpeedKmh
	case RoadTrunk:
		return defaultTrunkSpeedKmh
	case RoadPrimary:
		return defaultPrimarySpeedKmh
	case RoadSecondary:
		return defaultSecondarySpeedKmh
	case RoadTertiary:
		return defaultTertiarySpeedKmh
	case RoadResidential:
		return defaultResidentialSpeedKmh
	case RoadService:
		return defaultServiceSpeedKmh
	default:
		return defaultUnclassifiedSpeedKmh
	}
}

// Node is a routable graph vertex.
type Node struct {
	ID     NodeID
	Lat    float64 // WGS-84 latitude.
	Lon    float64 // WGS-84 longitude.
	X      float32 // Projected X in meters for region-local spatial calculations.
	Y      float32 // Projected Y in meters for region-local spatial calculations.
	CellID CellID  // Owning partition cell.
}

const (
	FlagOneWay uint8 = 1 << 0
	FlagToll   uint8 = 1 << 1
)

// Edge is a directed edge in the base road graph.
type Edge struct {
	ID          EdgeID
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32 // Travel time in seconds.
	DistanceM  float32 // Physical length in meters.
	SpeedKmh   float32 // Speed used to compute Weight.
	RoadClass  uint8
	Flags      uint8
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }

// OverlayEdge is an edge in the two-level overlay graph. Cross-cell edges are
// copied from the base graph; shortcut edges summarize the best path inside one
// cell between two boundary nodes.
type OverlayEdge struct {
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32 // Travel time in seconds.
	DistanceM   float32 // Shortest-path distance in meters.
	IsCrossCell bool
}

// AdjacencyList stores outgoing base-graph edges in CSR form.
type AdjacencyList struct {
	Offsets []uint32
	EdgeIDs []EdgeID
}

// Neighbours returns all outgoing edges for the internal node index.
func (a *AdjacencyList) Neighbours(idx uint32) []EdgeID {
	return a.EdgeIDs[a.Offsets[idx]:a.Offsets[idx+1]]
}

// Cell is one partition region produced by recursive bisection.
type Cell struct {
	ID               CellID
	InternalNodeIdxs []uint32
	BoundaryNodeIdxs []uint32
}

// OverlayAdjList stores outgoing overlay edges in CSR form over boundary nodes.
type OverlayAdjList struct {
	Mu           *sync.RWMutex
	Offsets      []uint32
	OverlayEdges []OverlayEdge
}

// Neighbours returns all outgoing overlay edges for the boundary-node index.
func (o *OverlayAdjList) Neighbours(idx uint32) []OverlayEdge {
	return o.OverlayEdges[o.Offsets[idx]:o.Offsets[idx+1]]
}

// BoundingBox holds the geographic extent of the map.
type BoundingBox struct {
	MinLat float64 `json:"min_lat"`
	MaxLat float64 `json:"max_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLon float64 `json:"max_lon"`
}

// IsZero reports whether the bounding box was never initialized.
func (b BoundingBox) IsZero() bool {
	return b.MinLat == 0 && b.MaxLat == 0 && b.MinLon == 0 && b.MaxLon == 0
}

// CenterLat returns the latitude midpoint of the bounding box.
func (b BoundingBox) CenterLat() float64 { return (b.MinLat + b.MaxLat) / 2 }

// CenterLon returns the longitude midpoint of the bounding box.
func (b BoundingBox) CenterLon() float64 { return (b.MinLon + b.MaxLon) / 2 }

// NominatimViewBox returns the Nominatim-style viewbox string "minLon,maxLat,maxLon,minLat".
func (b BoundingBox) NominatimViewBox() string {
	return fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", b.MinLon, b.MaxLat, b.MaxLon, b.MinLat)
}

// Graph contains the base graph, partition metadata, and overlay graph.
type Graph struct {
	Nodes   []Node
	Edges   []Edge
	BaseAdj AdjacencyList
	Cells   []Cell

	BoundaryNodeIdx  []int32
	BoundaryBaseIdxs []uint32

	OverlayAdj OverlayAdjList

	BBox             BoundingBox
	ProjectionRefLat float64
}

// BuildNodeIdxMap returns a map from OSM NodeID to internal node index.
func (g *Graph) BuildNodeIdxMap() map[NodeID]uint32 {
	m := make(map[NodeID]uint32, len(g.Nodes))
	for i, n := range g.Nodes {
		m[n.ID] = uint32(i)
	}
	return m
}
