package routing

// Step is one node in a route, describing the geographic position, which edge
// leads to this node, and the cumulative cost metrics up to this point.
type Step struct {
	NodeID      uint64  `json:"node_id"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	EdgeID      *uint32 `json:"edge_id,omitempty"` // nil on the first step (no incoming edge)
	DistanceM   float32 `json:"distance_m"`        // cumulative distance to this step
	BaseTimeSec float32 `json:"base_time_sec"`     // cumulative time using live weights
}

// Route is a complete source-to-destination path.
type Route struct {
	ID           string  `json:"id"` // UUIDv4
	Steps        []Step  `json:"steps"`
	TotalDistM   float32 `json:"total_dist_m"`
	TotalTimeSec float32 `json:"total_time_sec"` // computed with live weights at query time
}
