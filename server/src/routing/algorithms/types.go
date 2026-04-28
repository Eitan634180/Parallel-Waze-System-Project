package algorithms

import "nav-system/src/graph/model"

type BasePredecessor struct {
	PrevNodeIdx uint32
	EdgeID      model.EdgeID
}

type OverlayPredecessor struct {
	PrevNodeIdx uint32
	EdgeIdx     uint32
}

type Seed struct {
	NodeIdx uint32
	Cost    float32
}

type ijItem struct {
	idx  uint32
	cost float32
}

type astarItem struct {
	idx uint32
	f   float32
	g   float32
}

type localAstarItem struct {
	idx  uint32
	f    float32
	g    float32
	hops int
}
