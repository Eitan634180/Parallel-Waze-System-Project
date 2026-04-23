package simulation

import (
	"sync"

	"nav-system/src/graph/model"
	navigationsessions "nav-system/src/navigation/sessions"
	routingentities "nav-system/src/routing/entities"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/utilities"
)

type car struct {
	mu                     sync.Mutex
	id                     string
	route                  routingentities.Route
	routeRevision          uint64
	stepIdx                int
	progressM              float32
	edgeTimeS              float32
	lastObservationSampleS float32
	lat                    float64
	lon                    float64
	paceBias               float32
	edgeActive             bool
	lastSpeedKmh           float32
	pendingEdgeEvents      []EdgeTravel
	session                *navigationsessions.Session
	removed                bool
}

type EdgeTravel struct {
	EdgeID      uint32
	ObservedSec float32
}

type carRef struct {
	id  string
	car *car
}

func advanceCar(c *car, g *model.Graph, store *trafficstore.Store, dtSec float32, processPing func(sess *navigationsessions.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)) bool {
	remainingSec := dtSec

	for remainingSec > 0 {
		if c.stepIdx >= len(c.route.Steps) {
			return false
		}

		speedMps := currentSpeedMps(c, g, store)
		if speedMps <= 0 {
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return true
		}
		c.lastSpeedKmh = speedMps * utilities.KilometersPerHourToMps

		prev := c.route.Steps[c.stepIdx-1]
		cur := c.route.Steps[c.stepIdx]
		legDist := cur.DistanceM - prev.DistanceM
		if legDist <= 0 {
			legDist = MinLegDistanceFallbackM
		}
		leftOnLeg := legDist - c.progressM
		maxDistanceThisTick := speedMps * remainingSec
		if leftOnLeg > maxDistanceThisTick {
			c.progressM += maxDistanceThisTick
			c.edgeTimeS += remainingSec
			recordSimSpeedSample(c, cur, speedMps, g, store, c.session == nil)
			updateInterpolatedPosition(c, prev, cur, legDist)
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return true
		}

		timeOnLeg := leftOnLeg / speedMps
		c.edgeTimeS += timeOnLeg
		recordSimSpeedSample(c, cur, speedMps, g, store, c.session == nil)
		remainingSec -= timeOnLeg
		c.progressM = 0
		c.lat = cur.Lat
		c.lon = cur.Lon

		if c.edgeActive && cur.EdgeID != nil {
			eid := model.EdgeID(*cur.EdgeID)
			if c.session != nil {
				c.pendingEdgeEvents = append(c.pendingEdgeEvents, EdgeTravel{EdgeID: uint32(eid), ObservedSec: c.edgeTimeS})
			} else if edge, ok := g.Edge(eid); ok && edge.BaseWeight > 0 && c.edgeTimeS > 0 {
				store.RecordObservation(eid, c.edgeTimeS, edge.BaseWeight)
				store.LeaveEdge(eid)
			}
			c.edgeActive = false
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		}

		c.stepIdx++
		if c.stepIdx >= len(c.route.Steps) {
			if c.session != nil && processPing != nil {
				processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, len(c.route.Steps)-1, c.pendingEdgeEvents)
				c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
			}
			return false
		}
		if nextEdge := c.route.Steps[c.stepIdx].EdgeID; nextEdge != nil && c.session == nil {
			store.EnterEdge(model.EdgeID(*nextEdge))
			c.edgeActive = true
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		} else if nextEdge != nil {
			c.edgeActive = true
			c.edgeTimeS = 0
			c.lastObservationSampleS = 0
		}
	}

	if c.session != nil && processPing != nil {
		processPing(c.session, c.lat, c.lon, c.lastSpeedKmh, c.stepIdx, c.pendingEdgeEvents)
		c.pendingEdgeEvents = c.pendingEdgeEvents[:0]
	}
	return true
}

func recordSimSpeedSample(c *car, step routingentities.Step, speedMps float32, g *model.Graph, store *trafficstore.Store, enabled bool) {
	if !enabled {
		return
	}
	if !c.edgeActive || step.EdgeID == nil || speedMps <= 0 {
		return
	}
	if c.edgeTimeS < ObservationWarmupS || c.edgeTimeS-c.lastObservationSampleS < ObservationSampleIntervalS {
		return
	}

	eid := model.EdgeID(*step.EdgeID)
	edge, ok := g.Edge(eid)
	if !ok || edge.BaseWeight <= 0 || edge.DistanceM <= 0 {
		return
	}

	store.RecordSpeedSample(eid, speedMps*utilities.KilometersPerHourToMps, edge.BaseWeight, edge.DistanceM)
	c.lastObservationSampleS = c.edgeTimeS
}

func currentSpeedMps(c *car, g *model.Graph, store *trafficstore.Store) float32 {
	if c.stepIdx < 0 || c.stepIdx >= len(c.route.Steps) {
		return 0
	}
	stepIdx := c.stepIdx
	if stepIdx == 0 {
		if len(c.route.Steps) < 2 {
			return 0
		}
		stepIdx = 1
	}

	prev := c.route.Steps[stepIdx-1]
	cur := c.route.Steps[stepIdx]
	legDist := cur.DistanceM - prev.DistanceM
	legTime := cur.BaseTimeSec - prev.BaseTimeSec

	baseKmh := DefaultLegSpeedKmh
	if legDist > 0 && legTime > 0 {
		baseKmh = (legDist / legTime) * utilities.KilometersPerHourToMps
	}
	if cur.EdgeID != nil {
		eid := model.EdgeID(*cur.EdgeID)
		if edge, ok := g.Edge(eid); ok {
			recommended := store.RecommendedSpeedKmh(eid, edge.SpeedKmh, edge.DistanceM)
			if recommended > 0 {
				baseKmh = recommended
			} else if edge.SpeedKmh > 0 {
				baseKmh = edge.SpeedKmh
			}
		}
	}

	speedKmh := baseKmh * c.paceBias
	if speedKmh < MinSpeedKmh {
		speedKmh = MinSpeedKmh
	}
	if speedKmh > baseKmh*MaxSpeedMultiplier {
		speedKmh = baseKmh * MaxSpeedMultiplier
	}
	return speedKmh / utilities.KilometersPerHourToMps
}

func updateInterpolatedPosition(c *car, prev, cur routingentities.Step, legDist float32) {
	fraction := float64(c.progressM / legDist)
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	c.lat = prev.Lat + (cur.Lat-prev.Lat)*fraction
	c.lon = prev.Lon + (cur.Lon-prev.Lon)*fraction
}

func syncCarWithSession(c *car) {
	if c.session == nil {
		return
	}

	c.session.Mu.RLock()
	sessionRoute := c.session.Route
	sessionRouteRevision := c.session.RouteRevision
	sessionStepIdx := c.session.StepIdx
	sessionLat := c.session.LastLat
	sessionLon := c.session.LastLon
	c.session.Mu.RUnlock()

	if sessionRouteRevision != 0 && sessionRouteRevision != c.routeRevision {
		c.route = sessionRoute
		c.routeRevision = sessionRouteRevision
		c.stepIdx = sessionStepIdx
		c.progressM = 0
		c.edgeTimeS = 0
		c.lastObservationSampleS = 0
		c.edgeActive = sessionRoute.CurrentEdge(sessionStepIdx) != nil
	}

	c.lat = sessionLat
	c.lon = sessionLon
}
