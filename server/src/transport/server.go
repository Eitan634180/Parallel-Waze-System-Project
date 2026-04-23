package transport

import (
	"context"
	"net/http"
	"time"

	"nav-system/src/graph/model"
	navigationsessions "nav-system/src/navigation/sessions"
	navigationtracking "nav-system/src/navigation/tracking"
	navigationworkers "nav-system/src/navigation/workers"
	routingengine "nav-system/src/routing/engine"
	"nav-system/src/simulation"
	trafficstore "nav-system/src/traffic/store"
	handlers "nav-system/src/transport/handlers"
	transportweb "nav-system/src/transport/web"
)

// Server wires together all dependencies and exposes the HTTP mux.
type Server struct {
	g      *model.Graph
	store  *trafficstore.Store
	mgr    *navigationsessions.Manager
	router *routingengine.Router
	sim    *simulation.Manager

	routeCache  *handlers.RouteCache
	routes      *handlers.RouteHandler
	searches    *handlers.SearchHandler
	sessions    *handlers.SessionHandler
	simulations *handlers.SimulationHandler
	systems     *handlers.SystemHandler

	httpClient *http.Client
	mux        *http.ServeMux
}

// NewServer constructs the Server and registers all routes.
func NewServer(
	g *model.Graph,
	store *trafficstore.Store,
	mgr *navigationsessions.Manager,
	router *routingengine.Router,
	sim *simulation.Manager,
) *Server {
	s := &Server{
		g:          g,
		store:      store,
		mgr:        mgr,
		router:     router,
		sim:        sim,
		httpClient: &http.Client{Timeout: HTTPClientTimeout},
		mux:        http.NewServeMux(),
	}
	s.routeCache = handlers.NewRouteCache()
	s.routes = &handlers.RouteHandler{
		Graph:                   g,
		Store:                   store,
		Router:                  router,
		Cache:                   s.routeCache,
		SlowRequestLogThreshold: SlowRouteRequestLogThreshold,
		BaseRouteCount:          BaseRouteCount,
		MaxRouteCount:           MaxRouteCount,
	}
	searchConfig := LoadSearchConfigFromEnv()
	if searchConfig.ViewBox == "" && !g.BBox.IsZero() {
		searchConfig.ViewBox = g.BBox.NominatimViewBox()
	}
	s.searches = &handlers.SearchHandler{
		Client:                  s.httpClient,
		Limit:                   searchConfig.Limit,
		Language:                searchConfig.Language,
		CountryCodes:            searchConfig.CountryCodes,
		ViewBox:                 searchConfig.ViewBox,
		UpstreamURL:             searchConfig.UpstreamURL,
		UserAgent:               searchConfig.UserAgent,
		SlowRequestLogThreshold: SlowSearchRequestLogThreshold,
	}
	s.sessions = &handlers.SessionHandler{
		Manager:    mgr,
		RouteCache: s.routeCache,
		Tracker: &navigationtracking.Tracker{
			Graph:        g,
			Store:        store,
			Manager:      mgr,
			Router:       router,
			LiveWeights:  s.liveWeightFunc,
			PrepareRoute: s.routeCache.Store,
		},
		OriginAllowed:                 transportweb.IsAllowedBrowserOrigin,
		SlowSessionCreateLogThreshold: SlowSessionCreationLogThreshold,
	}
	s.simulations = &handlers.SimulationHandler{
		Sim:           sim,
		RouteCache:    s.routeCache,
		OriginAllowed: transportweb.IsAllowedBrowserOrigin,
	}
	s.systems = &handlers.SystemHandler{Graph: g}
	s.sim.SetSessionBridge(simulation.NewSessionBridge(s.sessions.Tracker))
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(transportweb.SearchRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.searches.Handle))
	s.mux.HandleFunc(transportweb.RouteRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.routes.Handle))
	s.mux.HandleFunc(transportweb.SessionRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.sessions.HandleCollection))
	s.mux.HandleFunc(transportweb.SessionSubtreeRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.sessions.HandleByID))
	s.mux.HandleFunc(transportweb.SimulationRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.simulations.HandleRoot))
	s.mux.HandleFunc(transportweb.SimulationRandomRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.simulations.HandleRandom))
	s.mux.HandleFunc(transportweb.SimulationWSRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.simulations.HandleWS))
	s.mux.HandleFunc(transportweb.SystemInfoRoutePath, transportweb.WithCORS(transportweb.IsAllowedBrowserOrigin, s.systems.Handle))
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// liveWeightFunc builds a WeightFunc backed by the live traffic Store.
func (s *Server) liveWeightFunc() routingengine.WeightFunc {
	return func(e *model.Edge) float32 {
		return s.store.LiveWeight(e.ID, e.BaseWeight)
	}
}

func (s *Server) RunOptimizationSweep(ctx context.Context) {
	navigationworkers.RunOptimizationSweep(ctx, s.mgr, s.g, s.store, s.router, s.liveWeightFunc(), s.routeCache.Store)
}

func (s *Server) RunRouteCacheGC(ctx context.Context) {
	ticker := time.NewTicker(RouteCacheGCInterval)
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
