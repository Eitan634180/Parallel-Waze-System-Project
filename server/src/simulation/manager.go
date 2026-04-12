package simulation

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/session"
	"nav-system/src/traffic"
)

const (
	tickInterval                  = 250 * time.Millisecond
	simObservationWarmupS         = float32(1.0)
	simObservationSampleIntervalS = float32(1.0)
	simRandomSeed                 = int64(42)
	simPaceBiasBase               = float32(0.85)
	simPaceBiasRange              = float32(0.30)
	minLegDistanceFallbackM       = float32(0.1)
	defaultLegSpeedKmh            = float32(50)
	minSimSpeedKmh                = float32(8)
	maxSimSpeedMultiplier         = float32(1.1)
)

type CarSnapshot struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type EdgeTravel struct {
	EdgeID      uint32
	ObservedSec float32
}

type SessionBridge struct {
	Create     func(route routing.Route, stepIdx int, lat, lon float64) *session.Session
	ProcessPing func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)
	Destroy    func(sess *session.Session)
}

type car struct {
	id         string
	route      routing.Route
	stepIdx    int
	progressM  float32
	edgeTimeS  float32
	lastObservationSampleS float32
	lat        float64
	lon        float64
	paceBias   float32
	edgeActive bool
	lastSpeedKmh float32
	pendingEdgeEvents []EdgeTravel
	session    *session.Session
}

type Manager struct {
	mu          sync.RWMutex
	g           *builder.Graph
	store       *traffic.Store
	rng         *rand.Rand
	nextID      int64
	cars        map[string]*car
	subscribers map[int]chan []CarSnapshot
	nextSubID   int
	sessionBridge SessionBridge
}

func NewManager(g *builder.Graph, store *traffic.Store) *Manager {
	return &Manager{
		g:           g,
		store:       store,
		rng:         rand.New(rand.NewSource(simRandomSeed)),
		cars:        make(map[string]*car),
		subscribers: make(map[int]chan []CarSnapshot),
	}
}

func (m *Manager) SetSessionBridge(bridge SessionBridge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionBridge = bridge
}

func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.Clear()
			return
		case <-ticker.C:
			m.tick(float32(tickInterval.Seconds()))
		}
	}
}

func (m *Manager) SpawnRoutes(routes []routing.Route, count int, minStep int) int {
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
			paceBias: simPaceBiasBase + m.rng.Float32()*simPaceBiasRange,
		}

		if m.sessionBridge.Create != nil && m.sessionBridge.ProcessPing != nil && m.sessionBridge.Destroy != nil {
			c.session = m.sessionBridge.Create(route, c.stepIdx, c.lat, c.lon)
			c.edgeActive = c.session != nil && route.Steps[startIndex+1].EdgeID != nil
		} else if route.Steps[startIndex+1].EdgeID != nil {
			m.store.EnterEdge(builder.EdgeID(*route.Steps[startIndex+1].EdgeID))
			c.edgeActive = true
		}
		m.cars[id] = c
		created++
	}
	return created
}

func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range m.cars {
		if c.session != nil && m.sessionBridge.Destroy != nil {
			m.sessionBridge.Destroy(c.session)
			continue
		}
		if c.edgeActive && c.stepIdx < len(c.route.Steps) && c.route.Steps[c.stepIdx].EdgeID != nil {
			m.store.LeaveEdge(builder.EdgeID(*c.route.Steps[c.stepIdx].EdgeID))
		}
	}
	clear(m.cars)
	m.broadcastLocked()
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.cars)
}

func (m *Manager) Subscribe() (int, <-chan []CarSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextSubID
	m.nextSubID++
	ch := make(chan []CarSnapshot, 1)
	m.subscribers[id] = ch
	ch <- m.snapshotLocked()
	return id, ch
}

func (m *Manager) Unsubscribe(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.subscribers[id]
	if !ok {
		return
	}
	delete(m.subscribers, id)
	close(ch)
}

func (m *Manager) tick(dtSec float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, c := range m.cars {
		if !advanceCar(c, m.g, m.store, dtSec, m.sessionBridge.ProcessPing) {
			if c.session != nil && m.sessionBridge.Destroy != nil {
				m.sessionBridge.Destroy(c.session)
			}
			delete(m.cars, id)
			continue
		}
		if c.session != nil {
			syncCarWithSession(c)
		}
	}
	m.broadcastLocked()
}

func (m *Manager) snapshotLocked() []CarSnapshot {
	out := make([]CarSnapshot, 0, len(m.cars))
	for _, c := range m.cars {
		out = append(out, CarSnapshot{ID: c.id, Lat: c.lat, Lon: c.lon})
	}
	return out
}

func (m *Manager) broadcastLocked() {
	if len(m.subscribers) == 0 {
		return
	}
	snapshot := m.snapshotLocked()
	for _, ch := range m.subscribers {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snapshot:
		default:
		}
	}
}

func advanceCar(c *car, g *builder.Graph, store *traffic.Store, dtSec float32, processPing func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)) bool {
	remainingSec := dtSec

	for remainingSec > 0 {
		if c.stepIdx >= len(c.route.Steps) {
			return false
		}

		speedMps := currentSpeedMps(c, g, store)
		if speedMps <= 0 {
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return true
		}
		c.lastSpeedKmh = speedMps * 3.6

		prev := c.route.Steps[c.stepIdx-1]
		cur := c.route.Steps[c.stepIdx]
		legDist := cur.DistanceM - prev.DistanceM
		if legDist <= 0 {
			legDist = minLegDistanceFallbackM
		}
		leftOnLeg := legDist - c.progressM
		maxDistanceThisTick := speedMps * remainingSec
		if leftOnLeg > maxDistanceThisTick {
			c.progressM += maxDistanceThisTick
			c.edgeTimeS += remainingSec
			recordSimSpeedSample(c, cur, speedMps, g, store, c.session == nil)
			updateInterpolatedPosition(c, prev, cur, legDist)
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return true
		}

		timeOnLeg := leftOnLeg / speedMps
		c.edgeTimeS += timeOnLeg
		recordSimSpeedSample(c, cur, speedMps, g, store, c.session == nil)
		remainingSec -= timeOnLeg
		c.progressM = 0
		c.lat = cur.Lat
		c.lon = cur.Lon

		if c.edgeActive && cur.EdgeID != nil {
			eid := builder.EdgeID(*cur.EdgeID)
			if c.session != nil {
				c.pendingEdgeEvents = append(c.pendingEdgeEvents, EdgeTravel{EdgeID: uint32(eid), ObservedSec: c.edgeTimeS})
			} else if int(eid) < len(g.Edges) && g.Edges[eid].Weight > 0 && c.edgeTimeS > 0 {
				store.RecordObservation(eid, c.edgeTimeS, g.Edges[eid].Weight)
				store.LeaveEdge(eid)
			}
			c.edgeActive = false
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		}

		c.stepIdx++
		if c.stepIdx >= len(c.route.Steps) {
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, len(c.route.Steps)-1, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return false
		}
		if nextEdge := c.route.Steps[c.stepIdx].EdgeID; nextEdge != nil && c.session == nil {
			store.EnterEdge(builder.EdgeID(*nextEdge))
			c.edgeActive = true
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		} else if nextEdge != nil {
			c.edgeActive = true
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		}
	}

	if c.session != nil && processPing != nil {
		processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
		c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
	}
	return true
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
		return minLegDistanceFallbackM
	}
	return legDist
}

func recordSimSpeedSample(c *car, step routing.Step, speedMps float32, g *builder.Graph, store *traffic.Store, enabled bool) {
	if !enabled {
		return
	}
	if !c.edgeActive || step.EdgeID == nil || speedMps <= 0 {
		return
	}
	if c.edgeTimeS < simObservationWarmupS || c.edgeTimeS-c.lastObservationSampleS < simObservationSampleIntervalS {
		return
	}

	eid := builder.EdgeID(*step.EdgeID)
	if int(eid) >= len(g.Edges) {
		return
	}
	edge := &g.Edges[eid]
	if edge.Weight <= 0 || edge.DistanceM <= 0 {
		return
	}

	store.RecordSpeedSample(eid, speedMps*3.6, edge.Weight, edge.DistanceM)
	c.lastObservationSampleS = c.edgeTimeS
}

func currentSpeedMps(c *car, g *builder.Graph, store *traffic.Store) float32 {
	if c.stepIdx < 0 || c.stepIdx >= len(c.route.Steps) {
		return 0
	}
	stepIdx := c.stepIdx
	if stepIdx == 0 {
		if len(c.route.Steps) < 2 {
			return 0
		}
		stepIdx = 1
	}

	prev := c.route.Steps[stepIdx-1]
	cur := c.route.Steps[stepIdx]
	legDist := cur.DistanceM - prev.DistanceM
	legTime := cur.BaseTimeSec - prev.BaseTimeSec

	baseKmh := defaultLegSpeedKmh
	if legDist > 0 && legTime > 0 {
		baseKmh = (legDist / legTime) * 3.6
	}
	if cur.EdgeID != nil {
		eid := builder.EdgeID(*cur.EdgeID)
		if int(eid) < len(g.Edges) {
			edge := g.Edges[eid]
			recommended := store.RecommendedSpeedKmh(eid, edge.SpeedKmh, edge.DistanceM)
			if recommended > 0 {
				baseKmh = recommended
			} else if edge.SpeedKmh > 0 {
				baseKmh = edge.SpeedKmh
			}
		}
	}

	speedKmh := baseKmh * c.paceBias
	if speedKmh < minSimSpeedKmh {
		speedKmh = minSimSpeedKmh
	}
	if speedKmh > baseKmh*maxSimSpeedMultiplier {
		speedKmh = baseKmh * maxSimSpeedMultiplier
	}
	return speedKmh / 3.6
}

func updateInterpolatedPosition(c *car, prev, cur routing.Step, legDist float32) {
	fraction := float64(c.progressM / legDist)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	c.lat = prev.Lat + (cur.Lat-prev.Lat)*fraction
	c.lon = prev.Lon + (cur.Lon-prev.Lon)*fraction
}

func carID(n int64) string {
	return "sim-car-" + strconv.FormatInt(n, 10)
}

func syncCarWithSession(c *car) {
	if c.session == nil {
		return
	}

	c.session.Mu.RLock()
	sessionRoute := c.session.Route
	sessionStepIdx := c.session.StepIdx
	sessionLat := c.session.LastLat
	sessionLon := c.session.LastLon
	c.session.Mu.RUnlock()

	if sessionRoute.ID != "" && sessionRoute.ID != c.route.ID {
		c.route = sessionRoute
		c.stepIdx = sessionStepIdx
		c.progressM = 0
		c.edgeTimeS = 0
		c.lastObservationSampleS = 0
		c.edgeActive = session.CurrentEdgeForStep(sessionRoute, sessionStepIdx) != nil
	}

	c.lat = sessionLat
	c.lon = sessionLon
}
