package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"nav-system/internal/routing"
	"nav-system/internal/session"
	"nav-system/internal/simulation"
	"nav-system/internal/traffic"
	"nav-system/map/builder"

	"github.com/google/uuid"
)

const (
	routeCacheTTL        = 30 * time.Minute
	routeCacheGCInterval = 5 * time.Minute
)

type routeCacheEntry struct {
	route     routing.Route
	createdAt time.Time
}

// Server wires together all dependencies and exposes the HTTP mux.
type Server struct {
	g      *builder.Graph
	store  *traffic.Store
	mgr    *session.Manager
	router *routing.Router
	sim    *simulation.Manager

	// Route cache: routes are computed by POST /route and stored here by ID so
	// that POST /session can look them up later.
	mu         sync.RWMutex
	routeCache map[string]routeCacheEntry

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
		routeCache: make(map[string]routeCacheEntry),
		httpClient: &http.Client{Timeout: 5 * time.Second},
		mux:        http.NewServeMux(),
	}
	s.mux.HandleFunc("/search", s.withCORS(s.handleSearch))
	s.mux.HandleFunc("/route", s.withCORS(s.handleRoute))
	s.mux.HandleFunc("/session", s.withCORS(s.handleSession))
	s.mux.HandleFunc("/session/", s.withCORS(s.handleSessionID))
	s.mux.HandleFunc("/simulation", s.withCORS(s.handleSimulation))
	s.mux.HandleFunc("/simulation/random", s.withCORS(s.handleSimulationRandom))
	s.mux.HandleFunc("/simulation/ws", s.withCORS(s.handleSimulationWS))
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// withCORS wraps a handler with permissive CORS headers.
func (s *Server) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
		return s.store.LiveWeight(e.ID, e.Weight, e.SpeedKmh, e.DistanceM)
	}
}

func (s *Server) prepareRoute(route routing.Route) routing.Route {
	s.mu.Lock()
	defer s.mu.Unlock()
	if route.ID == "" {
		route.ID = uuid.NewString()
	}
	s.routeCache[route.ID] = routeCacheEntry{
		route:     route,
		createdAt: time.Now(),
	}
	return route
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

func (s *Server) pruneExpiredRoutes(now time.Time) {
	cutoff := now.Add(-routeCacheTTL)

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, entry := range s.routeCache {
		if entry.createdAt.Before(cutoff) {
			delete(s.routeCache, id)
		}
	}
}
