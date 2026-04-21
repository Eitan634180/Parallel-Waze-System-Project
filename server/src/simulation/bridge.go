package simulation

import (
	navigationsession "nav-system/src/navigation/session"
	navigationtracker "nav-system/src/navigation/tracker"
	"nav-system/src/routing"
)

type SessionBridge struct {
	Create      func(route routing.Route, stepIdx int, lat, lon float64) *navigationsession.Session
	ProcessPing func(sess *navigationsession.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)
	Destroy     func(sess *navigationsession.Session)
}

func NewSessionBridge(tracker *navigationtracker.Tracker) SessionBridge {
	return SessionBridge{
		Create: func(route routing.Route, stepIdx int, lat, lon float64) *navigationsession.Session {
			return tracker.CreateHeadless(route, stepIdx, lat, lon)
		},
		ProcessPing: func(sess *navigationsession.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel) {
			observations := make([]navigationtracker.EdgeObservation, len(edgeEvents))
			for i, event := range edgeEvents {
				observations[i] = navigationtracker.EdgeObservation{
					EdgeID:      event.EdgeID,
					ObservedSec: event.ObservedSec,
				}
			}
			tracker.Advance(sess, lat, lon, speedKmh, stepIdx, observations)
		},
		Destroy: tracker.Destroy,
	}
}
