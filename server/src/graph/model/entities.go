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
	ID         EdgeID
	FromNode   uint32
	ToNode     uint32
	BaseWeight float32
	Length     float32
	SpeedLimit float32
	Flags      uint8
}

// OverlayEdge is an edge in the two-level overlay graph (cross-cell edges and shortcut edges).
type OverlayEdge struct {
	FromNode    uint32
	ToNode      uint32
	BaseWeight  float32
	StoreIdx    uint32
	IsCrossCell bool
}

// Cell is one partition region produced by recursive bisection.
type Cell struct {
	GateNodes []uint32
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }
