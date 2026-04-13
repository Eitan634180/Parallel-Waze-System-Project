package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/session"
	"nav-system/src/simulation"
	"nav-system/src/traffic"
)

const (
	routeCacheGCInterval = 5 * time.Minute
	apiHTTPClientTimeout = 5 * time.Second
)

// Server wires together all dependencies and exposes the HTTP mux.
type Server struct {
	g      *builder.Graph
	store  *traffic.Store
	mgr    *session.Manager
	router *routing.Router
	sim    *simulation.Manager
	search searchConfig

	routeCacheMu sync.RWMutex
	routeCache   map[string]routeCacheEntry

	httpClient *http.Client
	mux        *http.ServeMux
}

// NewServer constructs the Server and registers all routes.
func NewServer(
	g *builder.Graph,
	store *traffic.Store,
	mgr *session.Manager,
	router *routing.Router,
	sim *simulation.Manager,
) *Server {
	s := &Server{
		g:          g,
		store:      store,
		mgr:        mgr,
		router:     router,
		sim:        sim,
		search:     loadSearchConfig(),
		routeCache: make(map[string]routeCacheEntry),
		httpClient: &http.Client{Timeout: apiHTTPClientTimeout},
		mux:        http.NewServeMux(),
	}
	sim.SetSessionBridge(simulation.SessionBridge{
		Create: func(route routing.Route, stepIdx int, lat, lon float64) *session.Session {
			sess := mgr.CreateHeadless(route, stepIdx)
			sess.Mu.Lock()
			sess.LastLat = lat
			sess.LastLon = lon
			sess.Mu.Unlock()
			s.initializeSessionEdge(sess)
			return sess
		},
		ProcessPing: func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []simulation.EdgeTravel) {
			msg := pingMsg{
				Type:      wsMessageTypePing,
				Lat:       lat,
				Lon:       lon,
				SpeedKmh:  speedKmh,
				StepIndex: stepIdx,
			}
			if len(edgeEvents) > 0 {
				msg.EdgeEvents = make([]edgeTravel, len(edgeEvents))
				for i, event := range edgeEvents {
					msg.EdgeEvents[i] = edgeTravel{EdgeID: event.EdgeID, ObservedSec: event.ObservedSec}
				}
			}
			s.processPing(sess, msg)
		},
		Destroy: func(sess *session.Session) {
			sess.Mu.Lock()
			currentEdgeID := sess.CurrentEdgeID
			sess.CurrentEdgeID = nil
			sess.Mu.Unlock()
			if currentEdgeID != nil {
				s.store.LeaveEdge(builder.EdgeID(*currentEdgeID))
			}
			s.mgr.Delete(sess.ID)
		},
	})
	// Set Nominatim ViewBox from the graph's bounding box (if not overridden by env).
	if s.search.ViewBox == "" && !g.BBox.IsZero() {
		s.search.ViewBox = g.BBox.NominatimViewBox()
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(searchRoutePath, s.withCORS(s.handleSearch))
	s.mux.HandleFunc(routeRoutePath, s.withCORS(s.handleRoute))
	s.mux.HandleFunc(sessionRoutePath, s.withCORS(s.handleSession))
	s.mux.HandleFunc(sessionSubtreeRoutePath, s.withCORS(s.handleSessionID))
	s.mux.HandleFunc(simulationRoutePath, s.withCORS(s.handleSimulation))
	s.mux.HandleFunc(simulationRandomRoutePath, s.withCORS(s.handleSimulationRandom))
	s.mux.HandleFunc(simulationWSRoutePath, s.withCORS(s.handleSimulationWS))
	s.mux.HandleFunc(systemInfoRoutePath, s.withCORS(s.handleSystemInfo))
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// withCORS wraps a handler with permissive CORS headers.
func (s *Server) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get(headerOrigin)
		if origin != "" {
			if !isAllowedBrowserOrigin(origin) {
				http.Error(w, originNotAllowedMessage, http.StatusForbidden)
				return
			}
			w.Header().Set(headerAllowOrigin, origin)
			w.Header().Set(headerVary, headerOrigin)
			w.Header().Set(headerAllowMethods, corsAllowedMethods)
			w.Header().Set(headerAllowHeaders, corsAllowedHeaders)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

// liveWeightFunc builds a WeightFunc backed by the live traffic Store.
func (s *Server) liveWeightFunc() routing.WeightFunc {
	return func(e *builder.Edge) float32 {
		return s.store.LiveWeight(e.ID, e.Weight)
	}
}

func (s *Server) RunOptimizationSweep(ctx context.Context) {
	s.mgr.RunOptimizationSweep(ctx, s.g, s.store, s.router, s.liveWeightFunc(), s.cacheRoute)
}

func (s *Server) RunRouteCacheGC(ctx context.Context) {
	ticker := time.NewTicker(routeCacheGCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pruneExpiredRoutes(time.Now())
		}
	}
}
