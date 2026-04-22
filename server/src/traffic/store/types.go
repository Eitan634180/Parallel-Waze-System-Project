package store

import (
	"sync"

	"nav-system/src/graph/model"
)

// Store holds the live traffic state for every edge that has been observed.
// The zero value is valid via lazy ensure allocations; use NewStore() for clarity.
type Store struct {
	mu          sync.RWMutex
	weight      []float32 // observed EWMA multiplier (1.0 = free-flow)
	density     []int     // number of active sessions currently on edge
	prev        []float32 // multiplier at last propagation snapshot
	prevDensity []int     // density at last propagation snapshot

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
		s.resize(edgeCount)
	}
	return s
}

func (s *Store) ensure(id model.EdgeID) {
	if int(id) < len(s.weight) {
		return
	}

	newLen := int(id)*storeGrowthMultiplier + storeGrowthPadding
	s.resize(newLen)
}

func (s *Store) resize(newLen int) {
	if newLen <= len(s.weight) {
		return
	}

	newWeight := make([]float32, newLen)
	for i := range newWeight {
		newWeight[i] = 1.0
	}
	copy(newWeight, s.weight)
	s.weight = newWeight

	newPrev := make([]float32, newLen)
	for i := range newPrev {
		newPrev[i] = 1.0
	}
	copy(newPrev, s.prev)
	s.prev = newPrev

	newDensity := make([]int, newLen)
	copy(newDensity, s.density)
	s.density = newDensity

	newPrevDensity := make([]int, newLen)
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
