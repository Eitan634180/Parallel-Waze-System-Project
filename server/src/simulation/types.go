package simulation

type CarSnapshot struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type EdgeTravel struct {
	EdgeID      uint32
	ObservedSec float32
}
