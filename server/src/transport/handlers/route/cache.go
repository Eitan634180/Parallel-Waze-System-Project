package routehandler

import (
	"sync"
	"time"

	"nav-system/src/routing"

	"github.com/google/uuid"
)

const routeCacheTTL = 30 * time.Minute

type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	route     routing.Route
	createdAt time.Time
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

func (c *Cache) Store(route routing.Route) routing.Route {
	c.mu.Lock()
	defer c.mu.Unlock()

	if route.ID == "" {
		route.ID = uuid.NewString()
	}
	c.entries[route.ID] = cacheEntry{
		route:     route,
		createdAt: time.Now(),
	}
	return route
}

func (c *Cache) Get(id string) (routing.Route, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[id]
	return entry.route, ok
}

func (c *Cache) GetMany(ids []string) []routing.Route {
	c.mu.RLock()
	defer c.mu.RUnlock()

	routes := make([]routing.Route, 0, len(ids))
	for _, id := range ids {
		entry, ok := c.entries[id]
		if ok {
			routes = append(routes, entry.route)
		}
	}
	return routes
}

func (c *Cache) PruneExpired(now time.Time) {
	cutoff := now.Add(-routeCacheTTL)

	c.mu.Lock()
	defer c.mu.Unlock()

	for id, entry := range c.entries {
		if entry.createdAt.Before(cutoff) {
			delete(c.entries, id)
		}
	}
}
