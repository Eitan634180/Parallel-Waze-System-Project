package api

import (
	"context"
	"net/http"
	"time"

	apihttp "nav-system/src/api/http"
	routehandler "nav-system/src/api/handlers/route"
	searchhandler "nav-system/src/api/handlers/search"
	sessionhandler "nav-system/src/api/handlers/session"
	simulationhandler "nav-system/src/api/handlers/simulation"
	systemhandler "nav-system/src/api/handlers/system"
	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	routingmonitor "nav-system/src/routing/monitor"
	"nav-system/src/session"
	sessionruntime "nav-system/src/session/runtime"
	"nav-system/src/simulation"
	trafficstore "nav-system/src/traffic/store"
)

// Server wires together all dependencies and exposes the HTTP mux.
type Server struct {
	g      *model.Graph
	store  *trafficstore.Store
	mgr    *session.Manager
	router *routing.Router
	sim    *simulation.Manager

	routeCache  *routehandler.Cache
	routes      *routehandler.Handler
	searches    *searchhandler.Handler
	sessions    *sessionhandler.Handler
	simulations *simulationhandler.Handler
	systems     *systemhandler.Handler

	httpClient *http.Client
	mux        *http.ServeMux
}

// NewServer constructs the Server and registers all routes.
func NewServer(
	g *model.Graph,
	store *trafficstore.Store,
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
		httpClient: &http.Client{Timeout: coreconfig.APIHTTPClientTimeout},
		mux:        http.NewServeMux(),
	}
	s.routeCache = routehandler.NewCache()
	s.routes = &routehandler.Handler{
		Graph:  g,
		Store:  store,
		Router: router,
		Cache:  s.routeCache,
	}
	searchConfig := coreconfig.LoadSearchConfigFromEnv()
	if searchConfig.ViewBox == "" && !g.BBox.IsZero() {
		searchConfig.ViewBox = g.BBox.NominatimViewBox()
	}
	s.searches = &searchhandler.Handler{
		Client: s.httpClient,
		Config: searchConfig,
	}
	s.sessions = &sessionhandler.Handler{
		Manager:       mgr,
		RouteCache:    s.routeCache,
		Runtime: &sessionruntime.Runtime{
			Graph:        g,
			Store:        store,
			Manager:      mgr,
			Router:       router,
			LiveWeights:  s.liveWeightFunc,
			PrepareRoute: s.routeCache.Store,
		},
		OriginAllowed: apihttp.IsAllowedBrowserOrigin,
	}
	s.simulations = &simulationhandler.Handler{
		Sim:           sim,
		RouteCache:    s.routeCache,
		OriginAllowed: apihttp.IsAllowedBrowserOrigin,
	}
	s.systems = &systemhandler.Handler{Graph: g}
	sim.SetSessionBridge(simulation.NewSessionBridge(s.sessions.Runtime))
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(apihttp.SearchRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.searches.Handle))
	s.mux.HandleFunc(apihttp.RouteRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.routes.Handle))
	s.mux.HandleFunc(apihttp.SessionRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.sessions.HandleCollection))
	s.mux.HandleFunc(apihttp.SessionSubtreeRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.sessions.HandleByID))
	s.mux.HandleFunc(apihttp.SimulationRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.simulations.HandleRoot))
	s.mux.HandleFunc(apihttp.SimulationRandomRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.simulations.HandleRandom))
	s.mux.HandleFunc(apihttp.SimulationWSRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.simulations.HandleWS))
	s.mux.HandleFunc(apihttp.SystemInfoRoutePath, apihttp.WithCORS(apihttp.IsAllowedBrowserOrigin, s.systems.Handle))
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// liveWeightFunc builds a WeightFunc backed by the live traffic Store.
func (s *Server) liveWeightFunc() routing.WeightFunc {
	return func(e *model.Edge) float32 {
		return s.store.LiveWeight(e.ID, e.Weight)
	}
}

func (s *Server) RunOptimizationSweep(ctx context.Context) {
	routingmonitor.RunOptimizationSweep(ctx, s.mgr, s.g, s.store, s.router, s.liveWeightFunc(), s.routeCache.Store)
}

func (s *Server) RunRouteCacheGC(ctx context.Context) {
	ticker := time.NewTicker(coreconfig.APIRouteCacheGCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.routeCache.PruneExpired(time.Now())
		}
	}
}

func (s *Server) WarmSearch(ctx context.Context) error {
	return s.searches.Warm(ctx)
}
