package model

type EdgeID = uint32 // Index in edges list
type CellID = uint32 // Index in cells list

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
	SrcNode    uint32
	DstNode    uint32
	BaseWeight float32
	Length     float32
	SpeedLimit float32
	Flags      uint8
}

// OverlayEdge is an edge in the two-level overlay graph (cross-cell edges and shortcut edges).
type OverlayEdge struct {
	SrcNode     uint32
	DstNode     uint32
	BaseWeight  float32
	StoreIdx    uint32
	IsCrossCell bool
}

// Cell is one partition region produced by recursive bisection.
type Cell struct {
	Gates []uint32
}

func (e *Edge) IsOneWay() bool { return e.Flags&FlagOneWay != 0 }
