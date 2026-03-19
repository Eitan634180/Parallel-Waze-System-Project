package session

import (
	"math"
	"time"

	"nav-system/internal/routing"
	"nav-system/internal/traffic"
	"nav-system/map/builder"
)

const (
	rerouteCooldown    = 15 * time.Second
	etaThrottle        = 5 * time.Second
	congestionMulti    = traffic.CongestionThreshold
	rerouteSpeedupMin  = float32(0.10) // 10% faster to justify reroute
	rerouteMinGainSec  = float32(180)  // reroute when the alternative saves at least 3 minutes
	offRouteDistM      = float32(50)   // metres from expected step to trigger off-route
	offRouteSanityMaxM = float32(5_000)
	offRouteWindow     = 2
	offRouteStrikes    = 2
)

// Check evaluates reroute conditions and pushes ETA / reroute events.
// g, store, mgr, and router must be the live server-wide instances.
func Check(
	s *Session,
	snapLat, snapLon float64, // already-snapped position of the client
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	now := time.Now()

	s.Mu.Lock()
	if now.Sub(s.LastETAPush) >= etaThrottle {
		eta := computeETALocked(s, g, store)
		s.ETA = eta
		s.LastETAPush = now
		s.Mu.Unlock()

		etaVal := eta
		_ = s.Send(OutMsg{Type: "eta_update", ETASec: &etaVal})
		s.Mu.Lock()
	}

	offRouteDistance := distanceFromExpectedPathMLocked(s, snapLat, snapLon, g)
	s.LastOffRouteDistanceM = offRouteDistance
	if offRouteDistance > offRouteSanityMaxM {
		s.OffRouteViolations = 0
	} else if offRouteDistance > offRouteDistM {
		s.OffRouteViolations++
	} else {
		s.OffRouteViolations = 0
	}

	congestionAhead, congestedEdges := congestionSummaryLocked(s, store, g)
	s.LastCongestionAhead = congestionAhead
	s.LastCongestedEdges = congestedEdges

	if now.Sub(s.LastReroute) < rerouteCooldown {
		s.Mu.Unlock()
		return
	}

	if offRouteDistance <= offRouteSanityMaxM && offRouteDistance > offRouteDistM && s.OffRouteViolations >= offRouteStrikes {
		s.Mu.Unlock()
		doReroute(s, snapLat, snapLon, g, store, mgr, router, wf, prepareRoute, now, "off_route", nil, nil)
		return
	}

	if congestionAhead {
		dst := s.Route.Steps[len(s.Route.Steps)-1]
		oldETA := computeETALocked(s, g, store)
		s.Mu.Unlock()

		newRoutes := router.Compute(snapLat, snapLon, dst.Lat, dst.Lon, 1, wf)
		if len(newRoutes) == 0 {
			return
		}

		newETA := newRoutes[0].TotalTimeSec
		etaGain := oldETA - newETA
		if oldETA > 0 &&
			(etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec) &&
			!sameRemainingRoute(s, newRoutes[0]) {
			newRoute := prepareRoute(newRoutes[0])

			s.Mu.RLock()
			currentEdgeID := s.CurrentEdgeID
			s.Mu.RUnlock()
			if currentEdgeID != nil {
				store.LeaveEdge(builder.EdgeID(*currentEdgeID))
			}

			mgr.UpdateRoute(s, newRoute)

			s.Mu.Lock()
			if s.CurrentEdgeID != nil {
				store.EnterEdge(builder.EdgeID(*s.CurrentEdgeID))
			}
			s.LastReroute = now
			s.LastRerouteReason = "traffic"
			s.OffRouteViolations = 0
			s.Mu.Unlock()

			routePayload := RoutePayload{
				ID:           newRoute.ID,
				Steps:        newRoute.Steps,
				TotalDistM:   newRoute.TotalDistM,
				TotalTimeSec: newRoute.TotalTimeSec,
			}

			reason := "traffic"
			oldETAVal := oldETA
			newETAVal := newETA
			_ = s.Send(OutMsg{
				Type:          "reroute",
				Route:         &routePayload,
				RerouteReason: &reason,
				OldETASec:     &oldETAVal,
				NewETASec:     &newETAVal,
			})
		}
		return
	}

	s.Mu.Unlock()
}

func sameRemainingRoute(s *Session, candidate routing.Route) bool {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	currentStepIdx := s.StepIdx
	if currentStepIdx < 0 {
		currentStepIdx = 0
	}
	candidateStepIdx := InitialStepIndex(candidate)

	currentEdges := remainingEdgeSequence(s.Route, currentStepIdx)
	candidateEdges := remainingEdgeSequence(candidate, candidateStepIdx)

	if len(candidateEdges) == 0 {
		return true
	}
	if len(candidateEdges) > len(currentEdges) {
		return false
	}

	offset := len(currentEdges) - len(candidateEdges)
	if offset > 2 {
		return false
	}

	for i := range candidateEdges {
		if currentEdges[offset+i] != candidateEdges[i] {
			return false
		}
	}
	return true
}

func remainingEdgeSequence(route routing.Route, stepIdx int) []uint32 {
	if stepIdx < 0 {
		stepIdx = 0
	}
	if stepIdx >= len(route.Steps) {
		return nil
	}

	edges := make([]uint32, 0, len(route.Steps)-stepIdx)
	for _, step := range route.Steps[stepIdx:] {
		if step.EdgeID != nil {
			edges = append(edges, *step.EdgeID)
		}
	}
	return edges
}

// computeETALocked sums live edge weights for all remaining steps.
func computeETALocked(s *Session, g *builder.Graph, store *traffic.Store) float32 {
	var total float32
	steps := s.remainingStepsLocked()

	for i, step := range steps {
		if step.EdgeID == nil {
			continue
		}
		eid := builder.EdgeID(*step.EdgeID)
		if int(eid) >= len(g.Edges) {
			continue
		}
		edge := g.Edges[eid]
		weight := store.LiveWeight(eid, edge.Weight, edge.SpeedKmh, edge.DistanceM)

		if i == 0 && s.StepIdx > 0 && s.StepIdx < len(s.Route.Steps) && edge.DistanceM > 0 {
			px, py := projectForCheck(s.LastLat, s.LastLon)
			node := g.NodeByID(builder.NodeID(step.NodeID))

			if node != nil {
				distLeft := distancePointToPoint(px, py, node.X, node.Y)
				if distLeft < edge.DistanceM {
					fraction := distLeft / edge.DistanceM
					weight *= fraction
				}
			}
		}
		total += weight
	}
	return total
}

// congestionSummaryLocked reports whether any remaining edge is congested and how many.
func congestionSummaryLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int) {
	count := 0
	for _, step := range s.remainingStepsLocked() {
		if step.EdgeID == nil {
			continue
		}
		eid := builder.EdgeID(*step.EdgeID)
		if int(eid) >= len(g.Edges) {
			continue
		}
		edge := g.Edges[eid]
		if store.LiveWeight(eid, edge.Weight, edge.SpeedKmh, edge.DistanceM) >= edge.Weight*congestionMulti {
			count++
		}
	}
	return count > 0, count
}

func doReroute(
	s *Session,
	lat, lon float64,
	g *builder.Graph,
	store *traffic.Store,
	mgr *Manager,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
	now time.Time,
	reason string,
	oldETA *float32,
	newETA *float32,
) {
	s.Mu.RLock()
	dst := s.Route.Steps[len(s.Route.Steps)-1]
	currentEdgeID := s.CurrentEdgeID
	s.Mu.RUnlock()

	newRoutes := router.Compute(lat, lon, dst.Lat, dst.Lon, 1, wf)
	if len(newRoutes) == 0 {
		return
	}

	newRoute := prepareRoute(newRoutes[0])
	if currentEdgeID != nil {
		store.LeaveEdge(builder.EdgeID(*currentEdgeID))
	}
	mgr.UpdateRoute(s, newRoute)

	s.Mu.Lock()
	if s.CurrentEdgeID != nil {
		store.EnterEdge(builder.EdgeID(*s.CurrentEdgeID))
	}
	s.LastReroute = now
	s.LastRerouteReason = reason
	s.OffRouteViolations = 0
	s.Mu.Unlock()

	routePayload := RoutePayload{
		ID:           newRoute.ID,
		Steps:        newRoute.Steps,
		TotalDistM:   newRoute.TotalDistM,
		TotalTimeSec: newRoute.TotalTimeSec,
	}

	_ = s.Send(OutMsg{
		Type:          "reroute",
		Route:         &routePayload,
		RerouteReason: &reason,
		OldETASec:     oldETA,
		NewETASec:     newETA,
	})
}

func distanceFromExpectedPathMLocked(s *Session, lat, lon float64, g *builder.Graph) float32 {
	if len(s.Route.Steps) == 0 {
		return 0
	}

	px, py := projectForCheck(lat, lon)

	best := distanceFromExpectedProjectionMLocked(s, g, px, py)
	if lon >= -90 && lon <= 90 && lat >= -180 && lat <= 180 {
		swappedX, swappedY := projectForCheck(lon, lat)
		swapped := distanceFromExpectedProjectionMLocked(s, g, swappedX, swappedY)
		if swapped < best {
			return swapped
		}
	}
	return best
}

func distanceFromExpectedProjectionMLocked(s *Session, g *builder.Graph, px, py float32) float32 {
	if len(s.Route.Steps) == 1 {
		step := s.Route.Steps[0]
		node := g.NodeByID(builder.NodeID(step.NodeID))
		if node == nil {
			return 0
		}
		return distancePointToPoint(px, py, node.X, node.Y)
	}
	if s.StepIdx < 0 || s.StepIdx >= len(s.Route.Steps) {
		step := s.Route.Steps[minInt(s.StepIdx, len(s.Route.Steps)-1)]
		node := g.NodeByID(builder.NodeID(step.NodeID))
		if node == nil {
			return 0
		}
		return distancePointToPoint(px, py, node.X, node.Y)
	}

	start := maxInt(1, s.StepIdx-offRouteWindow)
	end := minInt(len(s.Route.Steps)-1, s.StepIdx+offRouteWindow)
	best := minDistanceToSegmentRangeLocked(s, g, px, py, start, end)
	if best > offRouteDistM {
		fullBest := minDistanceToSegmentRangeLocked(s, g, px, py, 1, len(s.Route.Steps)-1)
		if fullBest >= 0 && (best < 0 || fullBest < best) {
			best = fullBest
		}
	}
	if best >= 0 {
		return best
	}
	return 0
}

func minDistanceToSegmentRangeLocked(s *Session, g *builder.Graph, px, py float32, start, end int) float32 {
	best := float32(-1)
	for idx := start; idx <= end; idx++ {
		prev := g.NodeByID(builder.NodeID(s.Route.Steps[idx-1].NodeID))
		next := g.NodeByID(builder.NodeID(s.Route.Steps[idx].NodeID))
		if prev == nil || next == nil {
			continue
		}
		dist := distancePointToSegment(px, py, prev.X, prev.Y, next.X, next.Y)
		if best < 0 || dist < best {
			best = dist
		}
	}
	return best
}

// projectForCheck reuses the same projection constants as snap.go.
// (Duplicated here to avoid a circular import; values are identical.)
const (
	earthRM = 6_371_000.0
	lat0D   = 31.5
)

var cosL0 = float32(math.Cos(31.5 * math.Pi / 180.0))

func projectForCheck(lat, lon float64) (x, y float32) {
	x = float32(lon*3.14159265358979/180.0) * cosL0 * earthRM
	y = float32(lat * 3.14159265358979 / 180.0 * earthRM)
	return
}

func distancePointToPoint(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

func distancePointToSegment(px, py, ax, ay, bx, by float32) float32 {
	abx := bx - ax
	aby := by - ay
	den := abx*abx + aby*aby
	if den == 0 {
		return distancePointToPoint(px, py, ax, ay)
	}

	t := ((px-ax)*abx + (py-ay)*aby) / den
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	closestX := ax + t*abx
	closestY := ay + t*aby
	return distancePointToPoint(px, py, closestX, closestY)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
