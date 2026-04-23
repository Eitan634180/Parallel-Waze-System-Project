package model

// Index in the list of edges
type EdgeID = uint32

// Index the the list of cells
type CellID = uint32

// Node is a routable graph vertex.
type Node struct {
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
	BaseWeight  float32
	DistanceM   float32
	SpeedKmh    float32
	Flags       uint8
}

// OverlayEdge is an edge in the two-level overlay graph (cross-cell edges and shortcut edges).
type OverlayEdge struct {
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32
}

// Cell is one partition region produced by recursive bisection.
type Cell struct {
	GateNodeIdxs []uint32
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }
