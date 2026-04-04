package builder

// RawNode holds the coordinates of an OSM node referenced by at least one routable way.
type RawNode struct {
	ID  uint64
	Lat float64
	Lon float64
}

// RawWay holds the parsed fields of an OSM way relevant for routing.
type RawWay struct {
	ID        uint64
	NodeRefs  []uint64
	RoadClass uint8
	IsOneWay  bool
	MaxSpeed  float32 // 0 means "use default for road class"
}

// ParseResult is returned by the PBF parser.
type ParseResult struct {
	Nodes map[uint64]*RawNode // keyed by OSM node ID
	Ways  []*RawWay
}
