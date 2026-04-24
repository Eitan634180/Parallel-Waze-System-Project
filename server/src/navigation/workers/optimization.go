package workers

import (
	"context"
	"log"
	"time"

	"nav-system/src/graph/model"
	"nav-system/src/navigation"
	navigationanalysis "nav-system/src/navigation/analysis"
	navigationreroute "nav-system/src/navigation/reroute"
	navigationsessions "nav-system/src/navigation/sessions"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
)

type optimizationContext struct {
	oldETA  float32
	route   routingentities.Route
	stepIdx int
	version navigationreroute.SessionVersion
}

func RunOptimizationSweep(
	ctx context.Context,
	mgr *navigationsessions.Manager,
	g *model.Graph,
	store *trafficstore.Store,
	router *routingengine.Router,
	wf routingengine.WeightFunc,
	prepareRoute func(routingentities.Route) routingentities.Route,
) {
	jobs := make(chan *navigationsessions.Session, navigation.OptimizationJobBufferSize)

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
			for _, sess := range mgr.ActiveSessions() {
				tryQueueOptimization(jobs, sess)
			}
		}
	}
}

func optimizationWorker(
	ctx context.Context,
	jobs <-chan *navigationsessions.Session,
	mgr *navigationsessions.Manager,
	g *model.Graph,
	store *trafficstore.Store,
	router *routingengine.Router,
	wf routingengine.WeightFunc,
	prepareRoute func(routingentities.Route) routingentities.Route,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case sess := <-jobs:
			if sess == nil || !beginOptimizationWork(sess) {
				continue
			}

			context, candidate, ok := optimizationCandidate(sess, g, store, router, wf)
			if !ok {
				finishOptimizationWork(sess)
				continue
			}

			newETA := candidate.TotalTimeSec
			if !shouldAcceptOptimizationCandidate(sess, candidate, context, newETA) {
				finishOptimizationWork(sess)
				continue
			}

			navigationreroute.ApplyRouteUpdate(
				sess,
				candidate,
				g,
				store,
				mgr,
				prepareRoute,
				time.Now(),
				navigation.RerouteReasonTrafficCleared,
				&context.oldETA,
				&newETA,
				&context.version,
			)
			finishOptimizationWork(sess)
		}
	}
}

func tryQueueOptimization(jobs chan<- *navigationsessions.Session, sess *navigationsessions.Session) {
	sess.Mu.Lock()
	if !sess.CheckBetterRoute || sess.OptimizationQueued {
		sess.Mu.Unlock()
		return
	}
	sess.OptimizationQueued = true
	sess.Mu.Unlock()

	select {
	case jobs <- sess:
	default:
		sess.Mu.Lock()
		sess.OptimizationQueued = false
		sess.Mu.Unlock()
		// log.Printf("session: optimization queue full, deferring session %s", sess.ID)
	}
}

func beginOptimizationWork(sess *navigationsessions.Session) bool {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	if !sess.CheckBetterRoute {
		sess.OptimizationQueued = false
		return false
	}

	sess.CheckBetterRoute = false
	return true
}

func finishOptimizationWork(sess *navigationsessions.Session) {
	sess.Mu.Lock()
	sess.OptimizationQueued = false
	sess.Mu.Unlock()
}

func optimizationCandidate(
	s *navigationsessions.Session,
	g *model.Graph,
	store *trafficstore.Store,
	router *routingengine.Router,
	wf routingengine.WeightFunc,
) (optimizationContext, routingentities.Route, bool) {
	s.Mu.Lock()
	destination, ok := s.Route.Destination()
	if !ok {
		s.Mu.Unlock()
		return optimizationContext{}, routingentities.Route{}, false
	}

	context := optimizationContext{
		oldETA:  navigationanalysis.ComputeETA(s.Route, s.StepIdx, s.LastLat, s.LastLon, g, store),
		route:   s.Route,
		stepIdx: s.StepIdx,
		version: navigationreroute.SessionVersion{StepIdx: s.StepIdx, RouteRevision: s.RouteRevision},
	}
	snapLat, snapLon := s.LastLat, s.LastLon
	s.Mu.Unlock()

	start := time.Now()
	routes := router.Compute(snapLat, snapLon, destination.Lat, destination.Lon, 1, wf)
	elapsed := time.Since(start)
	if elapsed >= navigation.SlowComputeLogThreshold {
		log.Printf("session: optimization compute slow (session=%s routes=%d duration=%s)", s.ID, len(routes), elapsed.Round(time.Millisecond))
	}
	if len(routes) == 0 {
		return optimizationContext{}, routingentities.Route{}, false
	}

	return context, routes[0], true
}

func shouldAcceptOptimizationCandidate(s *navigationsessions.Session, candidate routingentities.Route, context optimizationContext, newETA float32) bool {
	s.Mu.RLock()
	tooSoon := time.Since(s.LastReroute) < navigation.RerouteCooldown
	stale := s.StepIdx != context.version.StepIdx || s.RouteRevision != context.version.RouteRevision
	s.Mu.RUnlock()

	if tooSoon || stale {
		return false
	}

	if context.route.SameRemaining(context.stepIdx, candidate) {
		return false
	}

	etaGain := context.oldETA - newETA
	return context.oldETA > 0 &&
		(etaGain/context.oldETA >= navigation.RerouteSpeedupMin || etaGain >= navigation.RerouteMinGainSec)
}
