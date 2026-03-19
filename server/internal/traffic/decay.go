package traffic

import (
	"context"
	"time"
)

const (
	decayInterval  = 30 * time.Second
	decayFactor    = float32(0.95)
	decayTolerance = float32(0.05)
)

// Worker runs a background goroutine that periodically decays all dirty edge
// weights back toward their base value.
//
// It exits cleanly when ctx is cancelled.
func Worker(ctx context.Context, store *Store) {
	ticker := time.NewTicker(decayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.ApplyDecay(decayFactor, decayTolerance)
		}
	}
}
