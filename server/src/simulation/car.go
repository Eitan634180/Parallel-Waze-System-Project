package simulation

import (
	"sync"

	coreconfig "nav-system/src/core/config"
	"nav-system/src/graph/model"
	"nav-system/src/routing"
	"nav-system/src/routing/engine"
	"nav-system/src/session"
	trafficstore "nav-system/src/traffic/store"
)

type car struct {
	mu                     sync.Mutex
	id                     string
	route                  routing.Route
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
	session                *session.Session
	removed                bool
}

type carRef struct {
	id  string
	car *car
}

func advanceCar(c *car, g *model.Graph, store *trafficstore.Store, dtSec float32, processPing func(sess *session.Session, lat, lon float64, speedKmh float32, stepIdx int, edgeEvents []EdgeTravel)) bool {
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
		c.lastSpeedKmh = speedMps * 3.6

		prev := c.route.Steps[c.stepIdx-1]
		cur := c.route.Steps[c.stepIdx]
		legDist := cur.DistanceM - prev.DistanceM
		if legDist <= 0 {
			legDist = coreconfig.SimulationMinLegDistanceFallbackM
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
			} else if int(eid) < len(g.Edges) && g.Edges[eid].Weight > 0 && c.edgeTimeS > 0 {
				store.RecordObservation(eid, c.edgeTimeS, g.Edges[eid].Weight)
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

func recordSimSpeedSample(c *car, step routing.Step, speedMps float32, g *model.Graph, store *trafficstore.Store, enabled bool) {
	if !enabled {
		return
	}
	if !c.edgeActive || step.EdgeID == nil || speedMps <= 0 {
		return
	}
	if c.edgeTimeS < coreconfig.SimulationObservationWarmupS || c.edgeTimeS-c.lastObservationSampleS < coreconfig.SimulationObservationSampleIntervalS {
		return
	}

	eid := model.EdgeID(*step.EdgeID)
	if int(eid) >= len(g.Edges) {
		return
	}
	edge := &g.Edges[eid]
	if edge.Weight <= 0 || edge.DistanceM <= 0 {
		return
	}

	store.RecordSpeedSample(eid, speedMps*3.6, edge.Weight, edge.DistanceM)
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

	baseKmh := coreconfig.SimulationDefaultLegSpeedKmh
	if legDist > 0 && legTime > 0 {
		baseKmh = (legDist / legTime) * 3.6
	}
	if cur.EdgeID != nil {
		eid := model.EdgeID(*cur.EdgeID)
		if int(eid) < len(g.Edges) {
			edge := g.Edges[eid]
			recommended := store.RecommendedSpeedKmh(eid, edge.SpeedKmh, edge.DistanceM)
			if recommended > 0 {
				baseKmh = recommended
			} else if edge.SpeedKmh > 0 {
				baseKmh = edge.SpeedKmh
			}
		}
	}

	speedKmh := baseKmh * c.paceBias
	if speedKmh < coreconfig.SimulationMinSpeedKmh {
		speedKmh = coreconfig.SimulationMinSpeedKmh
	}
	if speedKmh > baseKmh*coreconfig.SimulationMaxSpeedMultiplier {
		speedKmh = baseKmh * coreconfig.SimulationMaxSpeedMultiplier
	}
	return speedKmh / 3.6
}

func updateInterpolatedPosition(c *car, prev, cur routing.Step, legDist float32) {
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
		c.edgeActive = engine.CurrentEdgeForStep(sessionRoute, sessionStepIdx) != nil
	}

	c.lat = sessionLat
	c.lon = sessionLon
}
