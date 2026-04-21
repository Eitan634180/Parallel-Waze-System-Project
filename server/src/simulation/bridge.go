package simulation

import (
	"nav-system/src/routing"
	"nav-system/src/session"
	sessionruntime "nav-system/src/session/runtime"
)

type SessionBridge struct {
	Create      func(route routing.Route, stepIdx int, lat, lon float64) *session.Session
	ProcessPing func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)
	Destroy     func(sess *session.Session)
}

func NewSessionBridge(runtime *sessionruntime.Runtime) SessionBridge {
	return SessionBridge{
		Create: func(route routing.Route, stepIdx int, lat, lon float64) *session.Session {
			return runtime.CreateHeadless(route, stepIdx, lat, lon)
		},
		ProcessPing: func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel) {
			observations := make([]sessionruntime.EdgeObservation, len(edgeEvents))
			for i, event := range edgeEvents {
				observations[i] = sessionruntime.EdgeObservation{
					EdgeID:      event.EdgeID,
					ObservedSec: event.ObservedSec,
				}
			}
			runtime.Advance(sess, lat, lon, speedKmh, stepIdx, observations)
		},
		Destroy: runtime.Destroy,
	}
}
