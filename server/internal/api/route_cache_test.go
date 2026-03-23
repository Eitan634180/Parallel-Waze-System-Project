package api

import (
	"testing"
	"time"

	"nav-system/internal/routing"
)

func TestPruneExpiredRoutes(t *testing.T) {
	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	server := &Server{
		routeCache: map[string]routeCacheEntry{
			"fresh": {
				route:     routing.Route{ID: "fresh"},
				createdAt: now.Add(-routeCacheTTL / 2),
			},
			"expired": {
				route:     routing.Route{ID: "expired"},
				createdAt: now.Add(-routeCacheTTL - time.Second),
			},
		},
	}

	server.pruneExpiredRoutes(now)

	if _, ok := server.routeCache["fresh"]; !ok {
		t.Fatal("expected fresh route to remain cached")
	}
	if _, ok := server.routeCache["expired"]; ok {
		t.Fatal("expected expired route to be removed from cache")
	}
}
