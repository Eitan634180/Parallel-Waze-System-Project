package api

import (
	"time"

	"nav-system/src/routing"

	"github.com/google/uuid"
)

const routeCacheTTL = 30 * time.Minute

type routeCacheEntry struct {
	route     routing.Route
	createdAt time.Time
}

func (s *Server) cacheRoute(route routing.Route) routing.Route {
	s.routeCacheMu.Lock()
	defer s.routeCacheMu.Unlock()

	if route.ID == "" {
		route.ID = uuid.NewString()
	}
	s.routeCache[route.ID] = routeCacheEntry{
		route:     route,
		createdAt: time.Now(),
	}
	return route
}

func (s *Server) cachedRoute(id string) (routing.Route, bool) {
	s.routeCacheMu.RLock()
	defer s.routeCacheMu.RUnlock()

	entry, ok := s.routeCache[id]
	return entry.route, ok
}

func (s *Server) cachedRoutes(ids []string) []routing.Route {
	s.routeCacheMu.RLock()
	defer s.routeCacheMu.RUnlock()

	routes := make([]routing.Route, 0, len(ids))
	for _, id := range ids {
		entry, ok := s.routeCache[id]
		if ok {
			routes = append(routes, entry.route)
		}
	}
	return routes
}

func (s *Server) pruneExpiredRoutes(now time.Time) {
	cutoff := now.Add(-routeCacheTTL)

	s.routeCacheMu.Lock()
	defer s.routeCacheMu.Unlock()

	for id, entry := range s.routeCache {
		if entry.createdAt.Before(cutoff) {
			delete(s.routeCache, id)
		}
	}
}
