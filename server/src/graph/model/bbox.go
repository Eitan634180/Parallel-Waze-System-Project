package model

import "fmt"

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
