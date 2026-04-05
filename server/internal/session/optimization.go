package session

import (
	"context"
	"log"
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

// RunOptimizationSweep reroutes sessions that were flagged by the propagation heuristic.
func (m *Manager) RunOptimizationSweep(
	ctx context.Context,
	g *builder.Graph,
	store *traffic.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	jobs := make(chan *Session, 256)

	for i := 0; i < optimizationWorkerLimit; i++ {
		go m.optimizationWorker(ctx, jobs, g, store, router, wf, prepareRoute)
	}

	ticker := newTicker(optimizationSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, session := range m.flaggedSessions() {
				select {
				case jobs <- session:
				default:
					log.Printf("session: optimization queue full, dropping session %s", session.ID)
				}
			}
		}
	}
}

func (m *Manager) optimizationWorker(
	ctx context.Context,
	jobs <-chan *Session,
	g *builder.Graph,
	store *traffic.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case session := <-jobs:
			oldETA, candidate, version, ok := optimizationCandidate(session, g, store, router, wf)
			if !ok {
				continue
			}

			newETA := candidate.TotalTimeSec
			if !shouldAcceptOptimizationCandidate(session, candidate, version, oldETA, newETA) {
				continue
			}

			reason := rerouteReasonTrafficCleared
			applyRouteUpdate(session, candidate, g, store, m, prepareRoute, now(), reason, &oldETA, &newETA, nil)
		}
	}
}

func (m *Manager) flaggedSessions() []*Session {
	sessions := m.activeSessions()
	flagged := make([]*Session, 0, len(sessions))

	for _, session := range sessions {
		session.Mu.Lock()
		if !session.CheckBetterRoute {
			session.Mu.Unlock()
			continue
		}

		session.CheckBetterRoute = false
		session.Mu.Unlock()
		flagged = append(flagged, session)
	}

	return flagged
}

// sessionVersion captures a snapshot of the session state before a long-running
// route computation. Used by shouldAcceptOptimizationCandidate to detect if the
// session has moved or been rerouted while the computation was in progress.
type sessionVersion struct {
	stepIdx int
	routeID string
}

func optimizationCandidate(
	s *Session,
	g *builder.Graph,
	store *traffic.Store,
	router *routing.Router,
	wf routing.WeightFunc,
) (float32, routing.Route, sessionVersion, bool) {
	s.Mu.Lock()
	destination, ok := routeDestination(s.Route)
	if !ok {
		s.Mu.Unlock()
		return 0, routing.Route{}, sessionVersion{}, false
	}

	oldETA := computeETALocked(s, g, store)
	snapLat, snapLon := s.LastLat, s.LastLon
	version := sessionVersion{stepIdx: s.StepIdx, routeID: s.Route.ID}
	s.Mu.Unlock()

	start := time.Now()
	routes := router.Compute(snapLat, snapLon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= slowComputeLogThreshold {
		log.Printf("session: optimization compute slow (session=%s routes=%d duration=%s)", s.ID, len(routes), elapsed.Round(time.Millisecond))
	}
	if len(routes) == 0 {
		return 0, routing.Route{}, version, false
	}

	return oldETA, routes[0], version, true
}

func shouldAcceptOptimizationCandidate(s *Session, candidate routing.Route, v sessionVersion, oldETA, newETA float32) bool {
	s.Mu.RLock()
	tooSoon := time.Since(s.LastReroute) < rerouteCooldown
	stale := s.StepIdx != v.stepIdx || s.Route.ID != v.routeID
	s.Mu.RUnlock()

	if tooSoon || stale {
		return false
	}

	if sameRemainingRoute(s, candidate) {
		return false
	}

	etaGain := oldETA - newETA
	return oldETA > 0 &&
		(etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec)
}
