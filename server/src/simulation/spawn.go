package simulation

import (
	"log"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"time"

	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
)

type geoBox struct {
	minLat float64
	maxLat float64
	minLon float64
	maxLon float64
}

const simulationBBoxMissingLog = "simulation: random spawn skipped because graph bounding box is unavailable"

func (m *Manager) SpawnRoutes(routes []routing.Route, count int, minStep int) int {
	count = normalizedCount(count)
	if len(routes) == 0 || count <= 0 {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	created := 0
	for i := 0; i < count; i++ {
		route := routes[m.rng.Intn(len(routes))]
		if len(route.Steps) < 2 {
			continue
		}
		m.nextID++
		id := carID(m.nextID)

		startIndex := weightedStartIndex(route, minStep, m.rng)
		c := &car{
			id:       id,
			route:    route,
			stepIdx:  startIndex + 1,
			lat:      route.Steps[startIndex].Lat,
			lon:      route.Steps[startIndex].Lon,
			paceBias: coreconfig.SimulationPaceBiasBase + m.rng.Float32()*coreconfig.SimulationPaceBiasRange,
		}

		if m.sessionBridge.Create != nil && m.sessionBridge.ProcessPing != nil && m.sessionBridge.Destroy != nil {
			c.session = m.sessionBridge.Create(route, c.stepIdx, c.lat, c.lon)
			if c.session != nil {
				c.session.Mu.RLock()
				c.routeRevision = c.session.RouteRevision
				c.session.Mu.RUnlock()
				c.edgeActive = route.Steps[startIndex+1].EdgeID != nil
			}
		} else if route.Steps[startIndex+1].EdgeID != nil {
			m.store.EnterEdge(model.EdgeID(*route.Steps[startIndex+1].EdgeID))
			c.edgeActive = true
		}

		m.cars[id] = c
		created++
	}
	return created
}

func (m *Manager) SpawnRandom(count int, minStep int) int {
	count = normalizedCount(count)
	if count <= 0 {
		return 0
	}

	routes := m.randomRoutes(count)
	return m.SpawnRoutes(routes, len(routes), minStep)
}

func normalizedCount(count int) int {
	if count < coreconfig.SimulationDefaultCount {
		return coreconfig.SimulationDefaultCount
	}
	if count > coreconfig.SimulationMaxCount {
		return coreconfig.SimulationMaxCount
	}
	return count
}

func (m *Manager) randomRoutes(count int) []routing.Route {
	if m.g.BBox.IsZero() {
		log.Printf(simulationBBoxMissingLog)
		return nil
	}

	start := time.Now()
	bbox := geoBox{
		minLat: m.g.BBox.MinLat,
		maxLat: m.g.BBox.MaxLat,
		minLon: m.g.BBox.MinLon,
		maxLon: m.g.BBox.MaxLon,
	}

	routes := m.computeRandomCandidates(bbox, count)
	if elapsed := time.Since(start); elapsed >= coreconfig.SimulationSlowRouteLogThreshold {
		log.Printf("[simulation] random routes requested=%d generated=%d duration=%s", count, len(routes), elapsed.Round(time.Millisecond))
	}
	return routes
}

func (m *Manager) computeRandomCandidates(bbox geoBox, limit int) []routing.Route {
	if limit <= 0 {
		return nil
	}

	attempts := limit * coreconfig.SimulationRandomRouteAttemptFactor
	workerCount := minInt(maxInt(runtime.GOMAXPROCS(0), coreconfig.SimulationMinWorkers), attempts)

	jobs := make(chan struct{}, workerCount)
	results := make(chan routing.Route, workerCount)
	stop := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for {
				select {
				case <-stop:
					return
				case _, ok := <-jobs:
					if !ok {
						return
					}
				}

				srcLat, srcLon := bbox.randomPoint(rng)
				dstLat, dstLon := bbox.randomNearbyPoint(rng, srcLat, srcLon)
				if distanceSquared(srcLat, srcLon, dstLat, dstLon) < coreconfig.SimulationMinRandomRouteDistanceSq {
					continue
				}

				computed := m.router.Compute(srcLat, srcLon, dstLat, dstLon, coreconfig.SimulationDefaultRouteAlternatives, m.weightFunc())
				if len(computed) == 0 {
					continue
				}
				select {
				case <-stop:
					return
				case results <- computed[0]:
				}
			}
		}(i)
	}

	go func() {
		defer close(jobs)
		for i := 0; i < attempts; i++ {
			select {
			case <-stop:
				return
			case jobs <- struct{}{}:
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

func weightedStartIndex(route routing.Route, minStep int, rng *rand.Rand) int {
	maxStart := len(route.Steps) - 2
	startIndex := minStep
	if startIndex > maxStart {
		startIndex = maxStart
	}
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex >= maxStart {
		return startIndex
	}

	totalWeight := float32(0)
	for i := startIndex; i <= maxStart; i++ {
		totalWeight += routeLegWeight(route, i)
	}
	if totalWeight <= 0 {
		return startIndex + rng.Intn(maxStart-startIndex+1)
	}

	target := rng.Float32() * totalWeight
	for i := startIndex; i <= maxStart; i++ {
		target -= routeLegWeight(route, i)
		if target <= 0 {
			return i
		}
	}
	return maxStart
}

func routeLegWeight(route routing.Route, startIndex int) float32 {
	if startIndex < 0 || startIndex+1 >= len(route.Steps) {
		return 0
	}
	legDist := route.Steps[startIndex+1].DistanceM - route.Steps[startIndex].DistanceM
	if legDist <= 0 {
		return coreconfig.SimulationMinLegDistanceFallbackM
	}
	return legDist
}

func carID(n int64) string {
	return "sim-car-" + strconv.FormatInt(n, 10)
}

func (b geoBox) randomPoint(rng *rand.Rand) (float64, float64) {
	lat := b.minLat + rng.Float64()*(b.maxLat-b.minLat)
	lon := b.minLon + rng.Float64()*(b.maxLon-b.minLon)
	return lat, lon
}

func (b geoBox) randomNearbyPoint(rng *rand.Rand, centerLat, centerLon float64) (float64, float64) {
	minLat := maxFloat64(b.minLat, centerLat-coreconfig.SimulationMaxCommuteDegrees)
	maxLat := minFloat64(b.maxLat, centerLat+coreconfig.SimulationMaxCommuteDegrees)
	minLon := maxFloat64(b.minLon, centerLon-coreconfig.SimulationMaxCommuteDegrees)
	maxLon := minFloat64(b.maxLon, centerLon+coreconfig.SimulationMaxCommuteDegrees)

	lat := minLat + rng.Float64()*(maxLat-minLat)
	lon := minLon + rng.Float64()*(maxLon-minLon)
	return lat, lon
}

func distanceSquared(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := lat1 - lat2
	dLon := lon1 - lon2
	return dLat*dLat + dLon*dLon
}

func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
