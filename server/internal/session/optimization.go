package session

import (
	"context"

	"nav-system/internal/routing"
	"nav-system/internal/traffic"
	"nav-system/map/builder"
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
	ticker := newTicker(optimizationSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweepOptimizations(g, store, router, wf, prepareRoute)
		}
	}
}

func (m *Manager) sweepOptimizations(
	g *builder.Graph,
	store *traffic.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	for _, session := range m.flaggedSessions() {
		oldETA, candidate, ok := optimizationCandidate(session, g, store, router, wf)
		if !ok {
			continue
		}

		newETA := candidate.TotalTimeSec
		if !shouldAcceptOptimizationCandidate(session, candidate, oldETA, newETA) {
			continue
		}

		reason := "traffic_cleared"
		applyRouteUpdate(session, candidate, g, store, m, prepareRoute, now(), reason, &oldETA, &newETA)
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

func optimizationCandidate(
	s *Session,
	g *builder.Graph,
	store *traffic.Store,
	router *routing.Router,
	wf routing.WeightFunc,
) (float32, routing.Route, bool) {
	s.Mu.Lock()
	destination, ok := routeDestination(s.Route)
	if !ok {
		s.Mu.Unlock()
		return 0, routing.Route{}, false
	}

	oldETA := computeETALocked(s, g, store)
	snapLat, snapLon := s.LastLat, s.LastLon
	s.Mu.Unlock()

	routes := router.Compute(snapLat, snapLon, destination.Lat, destination.Lon, 1, wf)
	if len(routes) == 0 {
		return 0, routing.Route{}, false
	}

	return oldETA, routes[0], true
}

func shouldAcceptOptimizationCandidate(s *Session, candidate routing.Route, oldETA, newETA float32) bool {
	etaGain := oldETA - newETA
	return oldETA > 0 &&
		(etaGain/oldETA >= rerouteSpeedupMin || etaGain >= rerouteMinGainSec) &&
		!sameRemainingRoute(s, candidate)
}
