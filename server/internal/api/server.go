package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/session"
	"nav-system/internal/simulation"
	"nav-system/internal/traffic"
)

const (
	routeCacheGCInterval = 5 * time.Minute
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
		httpClient: &http.Client{Timeout: 5 * time.Second},
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/search", s.withCORS(s.handleSearch))
	s.mux.HandleFunc("/route", s.withCORS(s.handleRoute))
	s.mux.HandleFunc("/session", s.withCORS(s.handleSession))
	s.mux.HandleFunc("/session/", s.withCORS(s.handleSessionID))
	s.mux.HandleFunc("/simulation", s.withCORS(s.handleSimulation))
	s.mux.HandleFunc("/simulation/random", s.withCORS(s.handleSimulationRandom))
	s.mux.HandleFunc("/simulation/ws", s.withCORS(s.handleSimulationWS))
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// withCORS wraps a handler with permissive CORS headers.
func (s *Server) withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !isAllowedBrowserOrigin(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
		return s.store.LiveWeight(e.ID, e.Weight, e.SpeedKmh, e.DistanceM)
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
