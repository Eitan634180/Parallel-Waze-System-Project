package simulation

import (
	"context"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/traffic"
)

const tickInterval = 250 * time.Millisecond

type CarSnapshot struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type car struct {
	id         string
	route      routing.Route
	stepIdx    int
	progressM  float32
	lat        float64
	lon        float64
	paceBias   float32
	edgeActive bool
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
}

func NewManager(g *builder.Graph, store *traffic.Store) *Manager {
	return &Manager{
		g:           g,
		store:       store,
		rng:         rand.New(rand.NewSource(42)),
		cars:        make(map[string]*car),
		subscribers: make(map[int]chan []CarSnapshot),
	}
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

		maxStart := len(route.Steps) - 2
		startIndex := minStep
		if startIndex > maxStart {
			startIndex = maxStart
		}
		if startIndex < 0 {
			startIndex = 0
		}

		if maxStart > startIndex {
			startIndex += m.rng.Intn(maxStart - startIndex + 1)
		}

		c := &car{
			id:       id,
			route:    route,
			stepIdx:  startIndex + 1,
			lat:      route.Steps[startIndex].Lat,
			lon:      route.Steps[startIndex].Lon,
			paceBias: 0.85 + m.rng.Float32()*0.30,
		}

		if route.Steps[startIndex+1].EdgeID != nil {
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
		if !advanceCar(c, m.g, m.store, dtSec) {
			delete(m.cars, id)
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

func advanceCar(c *car, g *builder.Graph, store *traffic.Store, dtSec float32) bool {
	remainingM := currentSpeedMps(c, g, store) * dtSec

	for remainingM > 0 {
		if c.stepIdx >= len(c.route.Steps) {
			return false
		}

		prev := c.route.Steps[c.stepIdx-1]
		cur := c.route.Steps[c.stepIdx]
		legDist := cur.DistanceM - prev.DistanceM
		if legDist <= 0 {
			legDist = 0.1
		}
		leftOnLeg := legDist - c.progressM
		if leftOnLeg > remainingM {
			c.progressM += remainingM
			updateInterpolatedPosition(c, prev, cur, legDist)
			return true
		}

		remainingM -= leftOnLeg
		c.progressM = 0
		c.lat = cur.Lat
		c.lon = cur.Lon

		if c.edgeActive && cur.EdgeID != nil {
			store.LeaveEdge(builder.EdgeID(*cur.EdgeID))
			c.edgeActive = false
		}

		c.stepIdx++
		if c.stepIdx >= len(c.route.Steps) {
			return false
		}
		if nextEdge := c.route.Steps[c.stepIdx].EdgeID; nextEdge != nil {
			store.EnterEdge(builder.EdgeID(*nextEdge))
			c.edgeActive = true
		}
	}

	return true
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

	baseKmh := float32(50)
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
	if speedKmh < 8 {
		speedKmh = 8
	}
	if speedKmh > baseKmh*1.1 {
		speedKmh = baseKmh * 1.1
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
