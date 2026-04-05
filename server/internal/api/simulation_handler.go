package api

import (
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"sync"
	"time"

	"nav-system/internal/routing"
)

const (
	defaultSimulationCount   = 1
	randomRouteAttemptFactor = 6
	minRandomRouteDistanceSq = 0.0004
	simulationWSLogPrefix    = "api: simulation websocket"
	simulationBBoxMissingLog = "api: random simulation skipped because graph bounding box is unavailable"
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
		log.Printf("%s upgrade failed: %v", simulationWSLogPrefix, err)
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

type randomRouteCandidate struct {
	srcLat float64
	srcLon float64
	dstLat float64
	dstLon float64
}

func (s *Server) randomSimulationRoutes(count int) []routing.Route {
	if s.g.BBox.IsZero() {
		log.Printf(simulationBBoxMissingLog)
		return nil
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Now()
	attempts := count * randomRouteAttemptFactor
	candidates := make([]randomRouteCandidate, 0, attempts)

	bbox := geoBox{
		minLat: s.g.BBox.MinLat,
		maxLat: s.g.BBox.MaxLat,
		minLon: s.g.BBox.MinLon,
		maxLon: s.g.BBox.MaxLon,
	}
	for i := 0; i < attempts; i++ {
		srcLat, srcLon := bbox.randomPoint(rng)
		dstLat, dstLon := bbox.randomPoint(rng)
		candidates = append(candidates, randomRouteCandidate{
			srcLat: srcLat,
			srcLon: srcLon,
			dstLat: dstLat,
			dstLon: dstLon,
		})
	}

	routes := s.computeRandomSimulationCandidates(candidates, count)
	logSlowOperation(
		slowSimulationRouteLogThreshold,
		start,
		"[api] random simulation routes requested=%d generated=%d",
		count,
		len(routes),
	)
	return routes
}

func (s *Server) computeRandomSimulationCandidates(candidates []randomRouteCandidate, limit int) []routing.Route {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}

	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(candidates) {
		workerCount = len(candidates)
	}

	liveWeights := s.liveWeightFunc()
	jobs := make(chan randomRouteCandidate, workerCount)
	results := make(chan routing.Route, workerCount)
	stop := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var candidate randomRouteCandidate
				var ok bool
				select {
				case <-stop:
					return
				case candidate, ok = <-jobs:
					if !ok {
						return
					}
				}

				if distanceSquared(candidate.srcLat, candidate.srcLon, candidate.dstLat, candidate.dstLon) < minRandomRouteDistanceSq {
					continue
				}

				computed := s.router.Compute(candidate.srcLat, candidate.srcLon, candidate.dstLat, candidate.dstLon, 1, liveWeights)
				if len(computed) == 0 {
					continue
				}
				select {
				case <-stop:
					return
				case results <- computed[0]:
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case <-stop:
				return
			case jobs <- candidate:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	routes := make([]routing.Route, 0, limit)
	for route := range results {
		if len(routes) >= limit {
			continue
		}

		routes = append(routes, route)
		if len(routes) == limit {
			close(stop)
		}
	}

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
