package simulation

import (
	"context"
	"math/rand"
	"sync"
	"time"

	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	trafficstore "nav-system/src/traffic/store"
)

type Manager struct {
	mu            sync.RWMutex
	g             *model.Graph
	store         *trafficstore.Store
	router        *routing.Router
	liveWeights   func() routing.WeightFunc
	rng           *rand.Rand
	nextID        int64
	cars          map[string]*car
	subscribers   map[int]chan []CarSnapshot
	nextSubID     int
	sessionBridge SessionBridge
}

func NewManager(g *model.Graph, store *trafficstore.Store, router *routing.Router, liveWeights func() routing.WeightFunc) *Manager {
	return &Manager{
		g:           g,
		store:       store,
		router:      router,
		liveWeights: liveWeights,
		rng:         rand.New(rand.NewSource(coreconfig.SimulationRandomSeed)),
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
	ticker := time.NewTicker(coreconfig.SimulationTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.Clear()
			return
		case <-ticker.C:
			m.tick(float32(coreconfig.SimulationTickInterval.Seconds()))
		}
	}
}

func (m *Manager) Clear() {
	cars := m.detachCars()
	for _, ref := range cars {
		c := ref.car
		c.mu.Lock()
		if c.removed {
			c.mu.Unlock()
			continue
		}
		c.removed = true
		sessionRef := c.session
		edgeActive := c.edgeActive
		stepIdx := c.stepIdx
		route := c.route
		c.mu.Unlock()

		if sessionRef != nil && m.sessionBridge.Destroy != nil {
			m.sessionBridge.Destroy(sessionRef)
			continue
		}
		if edgeActive && stepIdx < len(route.Steps) && route.Steps[stepIdx].EdgeID != nil {
			m.store.LeaveEdge(model.EdgeID(*route.Steps[stepIdx].EdgeID))
		}
	}

	m.mu.Lock()
	m.broadcastLocked()
	m.mu.Unlock()
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
	refs := m.carRefs()
	finished := make([]carRef, 0)

	for _, ref := range refs {
		c := ref.car
		c.mu.Lock()
		if c.removed {
			c.mu.Unlock()
			continue
		}

		alive := advanceCar(c, m.g, m.store, dtSec, m.sessionBridge.ProcessPing)
		if alive && c.session != nil {
			syncCarWithSession(c)
		}
		if !alive {
			c.removed = true
			finished = append(finished, ref)
		}
		c.mu.Unlock()
	}

	for _, ref := range finished {
		if ref.car.session != nil && m.sessionBridge.Destroy != nil {
			m.sessionBridge.Destroy(ref.car.session)
		}
	}

	m.mu.Lock()
	for _, ref := range finished {
		delete(m.cars, ref.id)
	}
	m.broadcastLocked()
	m.mu.Unlock()
}

func (m *Manager) snapshotLocked() []CarSnapshot {
	out := make([]CarSnapshot, 0, len(m.cars))
	for _, c := range m.cars {
		c.mu.Lock()
		if !c.removed {
			out = append(out, CarSnapshot{ID: c.id, Lat: c.lat, Lon: c.lon})
		}
		c.mu.Unlock()
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

func (m *Manager) carRefs() []carRef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	refs := make([]carRef, 0, len(m.cars))
	for id, c := range m.cars {
		refs = append(refs, carRef{id: id, car: c})
	}
	return refs
}

func (m *Manager) detachCars() []carRef {
	m.mu.Lock()
	defer m.mu.Unlock()

	refs := make([]carRef, 0, len(m.cars))
	for id, c := range m.cars {
		refs = append(refs, carRef{id: id, car: c})
	}
	clear(m.cars)
	return refs
}

func (m *Manager) weightFunc() routing.WeightFunc {
	if m.liveWeights != nil {
		return m.liveWeights()
	}
	return routing.BaseWeight
}
