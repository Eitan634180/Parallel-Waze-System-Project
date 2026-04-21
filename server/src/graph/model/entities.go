package model

// NodeRawID is an OSM node ID.
type NodeRawID = uint64

// EdgeID indexes into Graph.Edges.
type EdgeID = uint32

// CellID indexes into Graph.Cells.
type CellID = uint32

// Node is a routable graph vertex.
type Node struct {
	ID     NodeRawID
	Lat    float64
	Lon    float64
	X      float32
	Y      float32
	CellID CellID
}

// Edge is a directed edge in the base road graph.
type Edge struct {
	ID          EdgeID
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32
	DistanceM   float32
	SpeedKmh    float32
	RoadClass   uint8
	Flags       uint8
}

// OverlayEdge is an edge in the two-level overlay graph (cross-cell edges and shortcut edges).
type OverlayEdge struct {
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32
	DistanceM   float32
	IsCrossCell bool
}

// Cell is one partition region produced by recursive bisection.
type Cell struct {
	ID               CellID
	BoundaryNodeIdxs []uint32
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }
