package api

import (
	"log"
	"math/rand"
	"net/http"
	"time"

	"nav-system/internal/routing"
)

const (
	defaultSimulationCount   = 1
	randomRouteAttemptFactor = 6
	minRandomRouteDistanceSq = 0.0004
)

type simulationSpawnRequest struct {
	Count        int      `json:"count"`
	RouteIDs     []string `json:"route_ids"`
	MinStepIndex int      `json:"min_step_index"`
}

type simulationSpawnResponse struct {
	Created int `json:"created"`
	Active  int `json:"active"`
}

type simulationSnapshotMessage struct {
	Type string                 `json:"type"`
	Cars []simulationCarMessage `json:"cars"`
}

type simulationCarMessage struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (s *Server) handleSimulation(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.spawnSimulationCars(w, r)
	case http.MethodDelete:
		s.sim.Clear()
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleSimulationRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	req, ok := decodeJSON[simulationSpawnRequest](w, r)
	if !ok {
		return
	}

	count := normalizedSimulationCount(req.Count)
	routes := s.randomSimulationRoutes(count)
	created := s.sim.SpawnRoutes(routes, len(routes), 0)
	writeJSON(w, http.StatusOK, simulationSpawnResponse{Created: created, Active: s.sim.Count()})
}

func (s *Server) spawnSimulationCars(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[simulationSpawnRequest](w, r)
	if !ok {
		return
	}

	routes := s.cachedRoutes(req.RouteIDs)
	if len(routes) == 0 {
		http.Error(w, "no routes found", http.StatusNotFound)
		return
	}

	created := s.sim.SpawnRoutes(routes, normalizedSimulationCount(req.Count), req.MinStepIndex)
	writeJSON(w, http.StatusOK, simulationSpawnResponse{Created: created, Active: s.sim.Count()})
}

func (s *Server) handleSimulationWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("simulation websocket upgrade: %v", err)
		return
	}

	subID, ch := s.sim.Subscribe()
	defer func() {
		s.sim.Unsubscribe(subID)
		conn.Close()
	}()

	for snapshots := range ch {
		payload := simulationSnapshotMessage{
			Type: "snapshot",
			Cars: make([]simulationCarMessage, 0, len(snapshots)),
		}
		for _, car := range snapshots {
			payload.Cars = append(payload.Cars, simulationCarMessage{ID: car.ID, Lat: car.Lat, Lon: car.Lon})
		}
		if err := conn.WriteJSON(payload); err != nil {
			return
		}
	}
}

func normalizedSimulationCount(count int) int {
	if count < defaultSimulationCount {
		return defaultSimulationCount
	}
	return count
}

type geoBox struct {
	minLat float64
	maxLat float64
	minLon float64
	maxLon float64
}

var telAvivBounds = geoBox{
	minLat: 32.01,
	maxLat: 32.15,
	minLon: 34.74,
	maxLon: 34.88,
}

func (s *Server) randomSimulationRoutes(count int) []routing.Route {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Now()
	routes := make([]routing.Route, 0, count)
	attempts := count * randomRouteAttemptFactor
	for len(routes) < count && attempts > 0 {
		attempts--
		srcLat, srcLon := telAvivBounds.randomPoint(rng)
		dstLat, dstLon := telAvivBounds.randomPoint(rng)
		if distanceSquared(srcLat, srcLon, dstLat, dstLon) < minRandomRouteDistanceSq {
			continue
		}
		computed := s.router.Compute(srcLat, srcLon, dstLat, dstLon, 1, s.liveWeightFunc())
		if len(computed) == 0 {
			continue
		}
		routes = append(routes, computed[0])
	}
	logSlowOperation(
		slowSimulationRouteLogThreshold,
		start,
		"[api] random simulation routes requested=%d generated=%d",
		count,
		len(routes),
	)
	return routes
}

func (b geoBox) randomPoint(rng *rand.Rand) (float64, float64) {
	lat := b.minLat + rng.Float64()*(b.maxLat-b.minLat)
	lon := b.minLon + rng.Float64()*(b.maxLon-b.minLon)
	return lat, lon
}

func distanceSquared(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := lat1 - lat2
	dLon := lon1 - lon2
	return dLat*dLat + dLon*dLon
}
