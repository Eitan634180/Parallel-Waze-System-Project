package store

import (
	"sync"

	"nav-system/src/graph/model"
)

const (
	// ewmaAlpha controls how quickly observed travel-time changes affect weight.
	ewmaAlpha = float32(0.15)

	// partialSampleAlpha is used for weaker in-progress speed samples from pings.
	partialSampleAlpha = float32(0.08)

	// CongestionThreshold marks edges that should be treated as congested.
	CongestionThreshold = float32(1.5)

	// SignificantShift is the minimum multiplier change that triggers a speed update.
	SignificantShift = float32(0.10)

	// Hint-speed parameters are only used for speed-update guidance.
	hintJamDensityVehPerKm = float32(120)
	hintMinSpeedRatio      = float32(0.02)
	hintMinEdgeLengthKm    = float32(0.02)
	hintAlpha              = 1.0
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

func (s *Store) ensure(id model.EdgeID) {
	if int(id) < len(s.weight) {
		return
	}

	newLen := int(id)*2 + 1024

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
