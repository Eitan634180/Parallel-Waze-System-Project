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

		// --- OPTIMIZATION 1: Local Route Repair ---
		repairTriggered, repairStepIdx, congestedCost := checkLocalRepairTriggerLocked(s, store, g)
		var patchSteps []routing.Step
		var patchSuccessful bool

		if repairTriggered {
			srcNodeID := s.Route.Steps[repairStepIdx-1].NodeID
			dstNodeID := s.Route.Steps[repairStepIdx].NodeID

			// Max hops allowed for a fast local detour on the overlay graph
			const maxHops = 5

			s.Mu.Unlock()
			patchSteps, patchSuccessful = router.LocalRepairOverlay(srcNodeID, dstNodeID, congestedCost, maxHops, wf)
			s.Mu.Lock()
		}

		var newRoutes []routing.Route
		var isLocalPatch bool

		if patchSuccessful && len(patchSteps) > 1 {
			newRoute := rebuildPatchedRoute(s.Route, repairStepIdx, patchSteps)
			newRoutes = []routing.Route{newRoute}
			isLocalPatch = true
		} else {
			// Fallback to global A*
			s.Mu.Unlock()
			newRoutes = router.Compute(snapLat, snapLon, dst.Lat, dst.Lon, 1, wf)
			s.Mu.Lock()
		}

		if len(newRoutes) == 0 {
			s.Mu.Unlock()
			return
		}

		s.Mu.Unlock()

		newETA := newRoutes[0].TotalTimeSec
		etaGain := oldETA - newETA
		if isLocalPatch || (oldETA > 0 &&
			(etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec) &&
			!sameRemainingRoute(s, newRoutes[0])) {
			newRouteCandidate := newRoutes[0]
			newRouteCandidate.CongestionAhead, newRouteCandidate.CongestedEdges = RouteCongestionSummary(newRouteCandidate, store, g)
			newRoute := prepareRoute(newRouteCandidate)

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
			s.ETA = newETA
			s.LastReroute = now
			if isLocalPatch {
				s.LastRerouteReason = "local_patch"
			} else {
				s.LastRerouteReason = "traffic"
			}
			s.OffRouteViolations = 0
			s.Mu.Unlock()

			routePayload := RoutePayload{
				ID:              newRoute.ID,
				Steps:           newRoute.Steps,
				TotalDistM:      newRoute.TotalDistM,
				TotalTimeSec:    newRoute.TotalTimeSec,
				CongestionAhead: newRoute.CongestionAhead,
				CongestedEdges:  newRoute.CongestedEdges,
			}

			reason := s.LastRerouteReason
			oldETAVal := oldETA
			newETAVal := newETA
			_ = s.Send(OutMsg{
				Type:          "reroute",
				Route:         &routePayload,
				RerouteReason: &reason,
				OldETASec:     &oldETAVal,
				NewETASec:     &newETAVal,
			})
			sendCurrentSpeedHints(s, store, g)
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
	return routeCongestionSummary(s.remainingStepsLocked(), store, g)
}

func RouteCongestionSummary(route routing.Route, store *traffic.Store, g *builder.Graph) (bool, int) {
	return routeCongestionSummary(route.Steps, store, g)
}

func routeCongestionSummary(steps []routing.Step, store *traffic.Store, g *builder.Graph) (bool, int) {
	count := 0
	for _, step := range steps {
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

func sendCurrentSpeedHints(s *Session, store *traffic.Store, g *builder.Graph) {
	for _, edgeID32 := range s.RemainingEdges() {
		eid := builder.EdgeID(edgeID32)
		if int(eid) >= len(g.Edges) {
			continue
		}

		edge := g.Edges[eid]
		if edge.SpeedKmh <= 0 {
			continue
		}

		recSpeed := store.RecommendedSpeedKmh(eid, edge.SpeedKmh, edge.DistanceM)
		if recSpeed == edge.SpeedKmh && store.Density(eid) == 0 {
			continue
		}

		edgeIDVal := uint32(eid)
		recSpeedVal := recSpeed
		_ = s.Send(OutMsg{
			Type:                "speed_update",
			EdgeID:              &edgeIDVal,
			RecommendedSpeedKmh: &recSpeedVal,
		})
	}
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

	newRouteCandidate := newRoutes[0]
	newRouteCandidate.CongestionAhead, newRouteCandidate.CongestedEdges = RouteCongestionSummary(newRouteCandidate, store, g)
	newRoute := prepareRoute(newRouteCandidate)
	if currentEdgeID != nil {
		store.LeaveEdge(builder.EdgeID(*currentEdgeID))
	}
	mgr.UpdateRoute(s, newRoute)

	s.Mu.Lock()
	if s.CurrentEdgeID != nil {
		store.EnterEdge(builder.EdgeID(*s.CurrentEdgeID))
	}
	s.ETA = newRoute.TotalTimeSec
	s.LastReroute = now
	s.LastRerouteReason = reason
	s.OffRouteViolations = 0
	s.Mu.Unlock()

	routePayload := RoutePayload{
		ID:              newRoute.ID,
		Steps:           newRoute.Steps,
		TotalDistM:      newRoute.TotalDistM,
		TotalTimeSec:    newRoute.TotalTimeSec,
		CongestionAhead: newRoute.CongestionAhead,
		CongestedEdges:  newRoute.CongestedEdges,
	}

	_ = s.Send(OutMsg{
		Type:          "reroute",
		Route:         &routePayload,
		RerouteReason: &reason,
		OldETASec:     oldETA,
		NewETASec:     newETA,
	})
	sendCurrentSpeedHints(s, store, g)
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

func checkLocalRepairTriggerLocked(s *Session, store *traffic.Store, g *builder.Graph) (bool, int, float32) {
	for i := maxInt(1, s.StepIdx); i < len(s.Route.Steps); i++ {
		step := s.Route.Steps[i]
		if step.EdgeID == nil {
			continue
		}
		eid := builder.EdgeID(*step.EdgeID)
		if int(eid) >= len(g.Edges) {
			continue
		}
		edge := g.Edges[eid]
		liveWeight := store.LiveWeight(eid, edge.Weight, edge.SpeedKmh, edge.DistanceM)

		// Compound threshold: severe spike
		if liveWeight >= edge.Weight*2.0 && (liveWeight-edge.Weight) >= 60.0 {
			u := g.NodeByID(s.Route.Steps[i-1].NodeID)
			v := g.NodeByID(s.Route.Steps[i].NodeID)
			// Ensure it's explicitly a cross-cell edge
			if u != nil && v != nil && u.CellID != v.CellID {
				return true, i, liveWeight
			}
		}
	}
	return false, -1, 0
}

func rebuildPatchedRoute(oldRoute routing.Route, repairStepIdx int, patch []routing.Step) routing.Route {
	rawSteps := make([]routing.Step, 0, len(oldRoute.Steps)+len(patch))

	for i := 0; i < repairStepIdx; i++ {
		step := oldRoute.Steps[i]
		if i > 0 {
			step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
			step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		} else {
			step.DistanceM = 0
			step.BaseTimeSec = 0
		}
		rawSteps = append(rawSteps, step)
	}

	for i := 1; i < len(patch); i++ {
		rawSteps = append(rawSteps, patch[i])
	}

	for i := repairStepIdx + 1; i < len(oldRoute.Steps); i++ {
		step := oldRoute.Steps[i]
		step.DistanceM = oldRoute.Steps[i].DistanceM - oldRoute.Steps[i-1].DistanceM
		step.BaseTimeSec = oldRoute.Steps[i].BaseTimeSec - oldRoute.Steps[i-1].BaseTimeSec
		rawSteps = append(rawSteps, step)
	}

	var cumD, cumT float32
	for i := range rawSteps {
		if i > 0 {
			cumD += rawSteps[i].DistanceM
			cumT += rawSteps[i].BaseTimeSec
		}
		rawSteps[i].DistanceM = cumD
		rawSteps[i].BaseTimeSec = cumT
	}

	return routing.Route{
		ID:           oldRoute.ID,
		Steps:        rawSteps,
		TotalDistM:   cumD,
		TotalTimeSec: cumT,
	}
}
