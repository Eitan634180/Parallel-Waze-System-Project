package monitor

import (
	"context"
	"log"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationmanager "nav-system/src/navigation/manager"
	navigationreroute "nav-system/src/navigation/reroute"
	navigationsession "nav-system/src/navigation/session"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	trafficstore "nav-system/src/traffic/store"
)

func RunOptimizationSweep(
	ctx context.Context,
	mgr *navigationmanager.Manager,
	g *model.Graph,
	store *trafficstore.Store,
	router *routing.Router,
	wf routing.WeightFunc,
	prepareRoute func(routing.Route) routing.Route,
) {
	jobs := make(chan *navigationsession.Session, 256)

	for i := 0; i < navigation.OptimizationWorkerLimit; i++ {
		go optimizationWorker(ctx, jobs, mgr, g, store, router, wf, prepareRoute)
	}

	ticker := time.NewTicker(navigation.OptimizationSweepInterval)
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
	jobs <-chan *navigationsession.Session,
	mgr *navigationmanager.Manager,
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

			navigationreroute.ApplyRouteUpdate(sess, candidate, g, store, mgr, prepareRoute, time.Now(), navigation.RerouteReasonTrafficCleared, &oldETA, &newETA, nil)
		}
	}
}

func flaggedSessions(sessions []*navigationsession.Session) []*navigationsession.Session {
	flagged := make([]*navigationsession.Session, 0, len(sessions))

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
	s *navigationsession.Session,
	g *model.Graph,
	store *trafficstore.Store,
	router *routing.Router,
	wf routing.WeightFunc,
) (float32, routing.Route, navigationreroute.SessionVersion, bool) {
	s.Mu.Lock()
	destination, ok := engine.Destination(s.Route)
	if !ok {
		s.Mu.Unlock()
		return 0, routing.Route{}, navigationreroute.SessionVersion{}, false
	}

	oldETA := ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store)
	snapLat, snapLon := s.LastLat, s.LastLon
	version := navigationreroute.SessionVersion{StepIdx: s.StepIdx, RouteRevision: s.RouteRevision}
	s.Mu.Unlock()

	start := time.Now()
	routes := router.Compute(snapLat, snapLon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= navigation.SlowComputeLogThreshold {
		log.Printf("session: optimization compute slow (session=%s routes=%d duration=%s)", s.ID, len(routes), elapsed.Round(time.Millisecond))
	}
	if len(routes) == 0 {
		return 0, routing.Route{}, version, false
	}

	return oldETA, routes[0], version, true
}

func shouldAcceptOptimizationCandidate(s *navigationsession.Session, candidate routing.Route, v navigationreroute.SessionVersion, oldETA, newETA float32) bool {
	s.Mu.RLock()
	tooSoon := time.Since(s.LastReroute) < navigation.RerouteCooldown
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
		(etaGain/oldETA >= navigation.RerouteSpeedupMin || etaGain >= navigation.RerouteMinGainSec)
}
