package routing

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
	SettledBaseNodes    int64
	SettledOverlayNodes int64
	RelaxedBaseEdges    int64
	RelaxedOverlayEdges int64
}

func (s *SearchStats) recordSettledBaseNode() {
	if s != nil {
		s.SettledBaseNodes++
	}
}

func (s *SearchStats) recordSettledOverlayNode() {
	if s != nil {
		s.SettledOverlayNodes++
	}
}

func (s *SearchStats) recordRelaxedBaseEdge() {
	if s != nil {
		s.RelaxedBaseEdges++
	}
}

func (s *SearchStats) recordRelaxedOverlayEdge() {
	if s != nil {
		s.RelaxedOverlayEdges++
	}
}
