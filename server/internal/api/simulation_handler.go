package api

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"

	"nav-system/internal/routing"
)

const (
	telAvivMinLat = 32.01
	telAvivMaxLat = 32.15
	telAvivMinLon = 34.74
	telAvivMaxLon = 34.88
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
	Type string                   `json:"type"`
	Cars []map[string]interface{} `json:"cars"`
}

func (s *Server) handleSimulation(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.spawnSimulationCars(w, r)
	case http.MethodDelete:
		s.sim.Clear()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSimulationRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req simulationSpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}

	routes := s.generateRandomSimulationRoutes(req.Count)
	created := s.sim.SpawnRoutes(routes, len(routes), 0)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(simulationSpawnResponse{Created: created, Active: s.sim.Count()})
}

func (s *Server) spawnSimulationCars(w http.ResponseWriter, r *http.Request) {
	var req simulationSpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}

	routes := make([]routing.Route, 0, len(req.RouteIDs))
	s.mu.RLock()
	for _, id := range req.RouteIDs {
		if entry, ok := s.routeCache[id]; ok {
			routes = append(routes, entry.route)
		}
	}
	s.mu.RUnlock()
	if len(routes) == 0 {
		http.Error(w, "no routes found", http.StatusNotFound)
		return
	}

	created := s.sim.SpawnRoutes(routes, req.Count, req.MinStepIndex)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(simulationSpawnResponse{Created: created, Active: s.sim.Count()})
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
			Cars: make([]map[string]interface{}, 0, len(snapshots)),
		}
		for _, car := range snapshots {
			payload.Cars = append(payload.Cars, map[string]interface{}{
				"id":  car.ID,
				"lat": car.Lat,
				"lon": car.Lon,
			})
		}
		if err := conn.WriteJSON(payload); err != nil {
			return
		}
	}
}

func (s *Server) generateRandomSimulationRoutes(count int) []routing.Route {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	routes := make([]routing.Route, 0, count)
	attempts := count * 6
	for len(routes) < count && attempts > 0 {
		attempts--
		srcLat := telAvivMinLat + rng.Float64()*(telAvivMaxLat-telAvivMinLat)
		srcLon := telAvivMinLon + rng.Float64()*(telAvivMaxLon-telAvivMinLon)
		dstLat := telAvivMinLat + rng.Float64()*(telAvivMaxLat-telAvivMinLat)
		dstLon := telAvivMinLon + rng.Float64()*(telAvivMaxLon-telAvivMinLon)
		if distanceSquared(srcLat, srcLon, dstLat, dstLon) < 0.0004 {
			continue
		}
		computed := s.router.Compute(srcLat, srcLon, dstLat, dstLon, 1, s.liveWeightFunc())
		if len(computed) == 0 {
			continue
		}
		route := s.prepareRoute(computed[0])
		routes = append(routes, route)
	}
	return routes
}

func distanceSquared(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := lat1 - lat2
	dLon := lon1 - lon2
	return dLat*dLat + dLon*dLon
}
