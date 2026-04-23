package store

import (
	"math"
	"sync"
	"sync/atomic"

	"nav-system/src/graph/model"
	"nav-system/src/traffic"
)

// Store holds the live traffic state for every edge that has been observed.
type Store struct {
	metaMu sync.Mutex
	data   atomic.Pointer[storeData]
	overlay atomic.Pointer[overlayData]

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

type storeData struct {
	weight  []atomic.Uint32
	density []atomic.Int32
}

type overlayData struct {
	weight []atomic.Uint32
}

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

func (s *Store) ensureLocked(id model.EdgeID) *storeData {
	data := s.data.Load()
	if data != nil && int(id) < len(data.weight) {
		return data
	}

	newLen := int(id)*traffic.StoreGrowthMultiplier + traffic.StoreGrowthPadding
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

func loadOverlayWeight(data *overlayData, edgeIdx uint32, fallback float32) float32 {
	if data == nil || int(edgeIdx) >= len(data.weight) {
		return fallback
	}
	return math.Float32frombits(data.weight[edgeIdx].Load())
}
