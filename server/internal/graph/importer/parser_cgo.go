//go:build cgo
// +build cgo

package importer

import (
	"fmt"

	"nav-system/internal/graph/builder"
)

// ParsePBF is intentionally disabled when cgo is enabled in this workspace.
// The project's map-build flow uses CGO_ENABLED=0 so the upstream osmpbf
// package falls back to the pure-Go zlib implementation.
func ParsePBF(path string) (*builder.ParseResult, error) {
	return nil, fmt.Errorf("ParsePBF requires CGO_ENABLED=0 in this project; rerun the map build with cgo disabled")
}
