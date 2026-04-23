package entities

import (
	"fmt"
	"strings"
)

type RoutingMode string

const (
	RoutingModeHierarchical RoutingMode = "hierarchical"
	RoutingModeBaseAStar    RoutingMode = "base-astar"
	RoutingModeBaseDijkstra RoutingMode = "base-dijkstra"
)

func ParseRoutingMode(raw string) (RoutingMode, error) {
	mode := RoutingMode(strings.TrimSpace(strings.ToLower(raw)))
	switch mode {
	case "", RoutingModeHierarchical:
		return RoutingModeHierarchical, nil
	case RoutingModeBaseAStar:
		return RoutingModeBaseAStar, nil
	case RoutingModeBaseDijkstra:
		return RoutingModeBaseDijkstra, nil
	default:
		return "", fmt.Errorf("unknown routing mode %q", raw)
	}
}

type SearchStats struct {
	VisitedNodes int64
}

func (s *SearchStats) RecordVisitedNode() {
	if s != nil {
		s.VisitedNodes++
	}
}
