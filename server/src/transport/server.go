package transport

import (
	"context"
	"net/http"
	"time"

	"nav-system/src/graph/model"
	navigationmanager "nav-system/src/navigation/manager"
	navigationmonitor "nav-system/src/navigation/monitor"
	navigationtracker "nav-system/src/navigation/tracker"
	"nav-system/src/routing"
	"nav-system/src/simulation"
	trafficstore "nav-system/src/traffic/store"
	routehandler "nav-system/src/transport/handlers/route"
	searchhandler "nav-system/src/transport/handlers/search"
	sessionhandler "nav-system/src/transport/handlers/session"
	simulationhandler "nav-system/src/transport/handlers/simulation"
	systemhandler "nav-system/src/transport/handlers/system"
	transporthttp "nav-system/src/transport/http"
)

// Server wires together all dependencies and exposes the HTTP mux.
type Server struct {
	g      *model.Graph
	store  *trafficstore.Store
	mgr    *navigationmanager.Manager
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
	mgr *navigationmanager.Manager,
	router *routing.Router,
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
	s.routeCache = routehandler.NewCache()
	s.routes = &routehandler.Handler{
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
	s.searches = &searchhandler.Handler{
		Client:                  s.httpClient,
		Limit:                   searchConfig.Limit,
		Language:                searchConfig.Language,
		CountryCodes:            searchConfig.CountryCodes,
		ViewBox:                 searchConfig.ViewBox,
		SlowRequestLogThreshold: SlowSearchRequestLogThreshold,
	}
	s.sessions = &sessionhandler.Handler{
		Manager:    mgr,
		RouteCache: s.routeCache,
		Tracker: &navigationtracker.Tracker{
			Graph:        g,
			Store:        store,
			Manager:      mgr,
			Router:       router,
			LiveWeights:  s.liveWeightFunc,
			PrepareRoute: s.routeCache.Store,
		},
		OriginAllowed:                 transporthttp.IsAllowedBrowserOrigin,
		SlowSessionCreateLogThreshold: SlowSessionCreationLogThreshold,
	}
	s.simulations = &simulationhandler.Handler{
		Sim:           sim,
		RouteCache:    s.routeCache,
		OriginAllowed: transporthttp.IsAllowedBrowserOrigin,
	}
	s.systems = &systemhandler.Handler{Graph: g}
	s.sim.SetSessionBridge(simulation.NewSessionBridge(s.sessions.Tracker))
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(transporthttp.SearchRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.searches.Handle))
	s.mux.HandleFunc(transporthttp.RouteRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.routes.Handle))
	s.mux.HandleFunc(transporthttp.SessionRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.sessions.HandleCollection))
	s.mux.HandleFunc(transporthttp.SessionSubtreeRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.sessions.HandleByID))
	s.mux.HandleFunc(transporthttp.SimulationRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.simulations.HandleRoot))
	s.mux.HandleFunc(transporthttp.SimulationRandomRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.simulations.HandleRandom))
	s.mux.HandleFunc(transporthttp.SimulationWSRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.simulations.HandleWS))
	s.mux.HandleFunc(transporthttp.SystemInfoRoutePath, transporthttp.WithCORS(transporthttp.IsAllowedBrowserOrigin, s.systems.Handle))
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
	navigationmonitor.RunOptimizationSweep(ctx, s.mgr, s.g, s.store, s.router, s.liveWeightFunc(), s.routeCache.Store)
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
