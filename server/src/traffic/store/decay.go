package store

import (
	"context"
	"time"

	coreconfig "nav-system/src/core/config"
)

// Worker runs a background goroutine that periodically decays all dirty edge
// weights back toward their base value.
//
// It exits cleanly when ctx is cancelled.
func Worker(ctx context.Context, store *Store) {
	ticker := time.NewTicker(coreconfig.TrafficDecayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.ApplyDecay(coreconfig.TrafficDecayFactor, coreconfig.TrafficDecayTolerance)
		}
	}
}
