package store

import (
	"math"
	"sync"
	"sync/atomic"

	"nav-system/src/graph/model"
)

type storeData struct {
	weight  []atomic.Uint32
	density []atomic.Int32
}

// Store holds the live traffic state for every edge that has been observed.
// The zero value is valid via lazy ensure allocations; use NewStore() for clarity.
type Store struct {
	metaMu sync.Mutex
	data   atomic.Pointer[storeData]

	prev        []float32 // multiplier at last propagation snapshot
	prevDensity []int32   // density at last propagation snapshot

	dirtyEdges    []model.EdgeID // persistent: edges whose multiplier != 1.0 (for decay)
	isDirty       []bool
	pendingEdges  []model.EdgeID // consumable: edges whose weights changed recently (for customization)
	isPending     []bool
	activityEdges []model.EdgeID // edges whose density changed since last snapshot
	isActive      []bool

	snapshotDedup []bool
}

// ChangedEdge describes an edge whose live state changed since the last snapshot.
type ChangedEdge struct {
	EdgeID        model.EdgeID
	OldMultiplier float32
	NewMultiplier float32
}

// NewStore creates an empty Store.
func NewStore() *Store {
	return &Store{}
}

func NewStoreWithCapacity(edgeCount int) *Store {
	s := &Store{}
	if edgeCount > 0 {
		s.resizeLocked(edgeCount)
	}
	return s
}

func newStoreData(edgeCount int) *storeData {
	data := &storeData{
		weight:  make([]atomic.Uint32, edgeCount),
		density: make([]atomic.Int32, edgeCount),
	}
	defaultWeight := math.Float32bits(1.0)
	for i := range data.weight {
		data.weight[i].Store(defaultWeight)
	}
	return data
}

func loadWeight(data *storeData, id model.EdgeID) float32 {
	if data == nil || int(id) >= len(data.weight) {
		return 1.0
	}
	return math.Float32frombits(data.weight[id].Load())
}

func loadDensity(data *storeData, id model.EdgeID) int32 {
	if data == nil || int(id) >= len(data.density) {
		return 0
	}
	return data.density[id].Load()
}

func storeWeight(data *storeData, id model.EdgeID, weight float32) {
	data.weight[id].Store(math.Float32bits(weight))
}

func (s *Store) ensureLocked(id model.EdgeID) *storeData {
	data := s.data.Load()
	if data != nil && int(id) < len(data.weight) {
		return data
	}

	newLen := int(id)*storeGrowthMultiplier + storeGrowthPadding
	if newLen <= int(id) {
		newLen = int(id) + 1
	}
	s.resizeLocked(newLen)
	return s.data.Load()
}

func (s *Store) resizeLocked(newLen int) {
	current := s.data.Load()
	currentLen := 0
	if current != nil {
		currentLen = len(current.weight)
	}
	if newLen <= currentLen {
		return
	}

	next := newStoreData(newLen)
	if current != nil {
		for i := 0; i < currentLen; i++ {
			next.weight[i].Store(current.weight[i].Load())
			next.density[i].Store(current.density[i].Load())
		}
	}
	s.data.Store(next)

	newPrev := make([]float32, newLen)
	for i := range newPrev {
		newPrev[i] = 1.0
	}
	copy(newPrev, s.prev)
	s.prev = newPrev

	newPrevDensity := make([]int32, newLen)
	copy(newPrevDensity, s.prevDensity)
	s.prevDensity = newPrevDensity

	newIsDirty := make([]bool, newLen)
	copy(newIsDirty, s.isDirty)
	s.isDirty = newIsDirty

	newIsPending := make([]bool, newLen)
	copy(newIsPending, s.isPending)
	s.isPending = newIsPending

	newIsActive := make([]bool, newLen)
	copy(newIsActive, s.isActive)
	s.isActive = newIsActive

	newSnapshotDedup := make([]bool, newLen)
	copy(newSnapshotDedup, s.snapshotDedup)
	s.snapshotDedup = newSnapshotDedup
}
