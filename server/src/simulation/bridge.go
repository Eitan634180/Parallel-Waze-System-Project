package simulation

import (
	navigationsessions "nav-system/src/navigation/sessions"
	navigationtracking "nav-system/src/navigation/tracking"
	routingentities "nav-system/src/routing/entities"
)

type SessionBridge struct {
	Create      func(route routingentities.Route, stepIdx int, lat, lon float64) *navigationsessions.Session
	ProcessPing func(sess *navigationsessions.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)
	Destroy     func(sess *navigationsessions.Session)
}

func NewSessionBridge(tracker *navigationtracking.Tracker) SessionBridge {
	return SessionBridge{
		Create: func(route routingentities.Route, stepIdx int, lat, lon float64) *navigationsessions.Session {
			return tracker.CreateHeadless(route, stepIdx, lat, lon)
		},
		ProcessPing: func(sess *navigationsessions.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel) {
			observations := make([]navigationtracking.EdgeObservation, len(edgeEvents))
			for i, event := range edgeEvents {
				observations[i] = navigationtracking.EdgeObservation{
					EdgeID:      event.EdgeID,
					ObservedSec: event.ObservedSec,
				}
			}
			tracker.Advance(sess, lat, lon, speedKmh, stepIdx, observations)
		},
		Destroy: tracker.Destroy,
	}
}
