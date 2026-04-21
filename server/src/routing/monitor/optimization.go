package monitor

import (
	"context"
	"log"
	"time"

	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	routingreroute "nav-system/src/routing/reroute"
	"nav-system/src/session"
	trafficstore "nav-system/src/traffic/store"
)

func RunOptimizationSweep(
	ctx context.Context,
	mgr *session.Manager,
	g *model.Graph,
	store *trafficstore.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	jobs := make(chan *session.Session, 256)

	for i := 0; i < coreconfig.RoutingOptimizationWorkerLimit; i++ {
		go optimizationWorker(ctx, jobs, mgr, g, store, router, wf, prepareRoute)
	}

	ticker := time.NewTicker(coreconfig.RoutingOptimizationSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, sess := range flaggedSessions(mgr.ActiveSessions()) {
				select {
				case jobs <- sess:
				default:
					log.Printf("session: optimization queue full, dropping session %s", sess.ID)
				}
			}
		}
	}
}

func optimizationWorker(
	ctx context.Context,
	jobs <-chan *session.Session,
	mgr *session.Manager,
	g *model.Graph,
	store *trafficstore.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case sess := <-jobs:
			oldETA, candidate, version, ok := optimizationCandidate(sess, g, store, router, wf)
			if !ok {
				continue
			}

			newETA := candidate.TotalTimeSec
			if !shouldAcceptOptimizationCandidate(sess, candidate, version, oldETA, newETA) {
				continue
			}

			routingreroute.ApplyRouteUpdate(sess, candidate, g, store, mgr, prepareRoute, time.Now(), rerouteReasonTrafficCleared, &oldETA, &newETA, nil)
		}
	}
}

func flaggedSessions(sessions []*session.Session) []*session.Session {
	flagged := make([]*session.Session, 0, len(sessions))

	for _, sess := range sessions {
		sess.Mu.Lock()
		if !sess.CheckBetterRoute {
			sess.Mu.Unlock()
			continue
		}

		sess.CheckBetterRoute = false
		sess.Mu.Unlock()
		flagged = append(flagged, sess)
	}

	return flagged
}

func optimizationCandidate(
	s *session.Session,
	g *model.Graph,
	store *trafficstore.Store,
	router *routing.Router,
	wf routing.WeightFunc,
) (float32, routing.Route, routingreroute.SessionVersion, bool) {
	s.Mu.Lock()
	destination, ok := engine.Destination(s.Route)
	if !ok {
		s.Mu.Unlock()
		return 0, routing.Route{}, routingreroute.SessionVersion{}, false
	}

	oldETA := ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store)
	snapLat, snapLon := s.LastLat, s.LastLon
	version := routingreroute.SessionVersion{StepIdx: s.StepIdx, RouteRevision: s.RouteRevision}
	s.Mu.Unlock()

	start := time.Now()
	routes := router.Compute(snapLat, snapLon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= coreconfig.RoutingSlowComputeLogThreshold {
		log.Printf("session: optimization compute slow (session=%s routes=%d duration=%s)", s.ID, len(routes), elapsed.Round(time.Millisecond))
	}
	if len(routes) == 0 {
		return 0, routing.Route{}, version, false
	}

	return oldETA, routes[0], version, true
}

func shouldAcceptOptimizationCandidate(s *session.Session, candidate routing.Route, v routingreroute.SessionVersion, oldETA, newETA float32) bool {
	s.Mu.RLock()
	tooSoon := time.Since(s.LastReroute) < coreconfig.RoutingRerouteCooldown
	stale := s.StepIdx != v.StepIdx || s.RouteRevision != v.RouteRevision
	s.Mu.RUnlock()

	if tooSoon || stale {
		return false
	}

	if engine.SameRemainingRoute(s.Route, s.StepIdx, candidate) {
		return false
	}

	etaGain := oldETA - newETA
	return oldETA > 0 &&
		(etaGain/oldETA >= coreconfig.RoutingRerouteSpeedupMin || etaGain >= coreconfig.RoutingRerouteMinGainSec)
}
