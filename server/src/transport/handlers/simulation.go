package handlers

import (
	"fmt"
	"log"
	"net/http"

	"nav-system/src/simulation"
	transportweb "nav-system/src/transport/web"
)

const simulationWSLogPrefix = "transport: simulation websocket"

type SimulationHandler struct {
	Sim           *simulation.Manager
	RouteCache    *RouteCache
	OriginAllowed func(string) bool
}

type simulationRequest struct {
	Count        int      `json:"count"`
	RouteIDs     []string `json:"route_ids"`
	MinStepIndex int      `json:"min_step_index"`
}

type simulationResponse struct {
	Created int `json:"created"`
	Active  int `json:"active"`
}

type simulationSnapshotMessage struct {
	Type string                 `json:"type"`
	Cars []simulationCarMessage `json:"cars"`
}

type simulationCarMessage struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (h *SimulationHandler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.spawnCars(w, r)
	case http.MethodDelete:
		h.Sim.Clear()
		w.WriteHeader(http.StatusNoContent)
	default:
		transportweb.MethodNotAllowed(w)
	}
}

func (h *SimulationHandler) HandleRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		transportweb.MethodNotAllowed(w)
		return
	}

	req, ok := transportweb.DecodeJSON[simulationRequest](w, r)
	if !ok {
		return
	}
	if err := req.validateRandom(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created := h.Sim.SpawnRandom(req.Count, req.MinStepIndex)
	transportweb.WriteJSON(w, http.StatusOK, simulationResponse{Created: created, Active: h.Sim.Count()})
}

func (h *SimulationHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	upgrader := transportweb.NewUpgrader(h.OriginAllowed)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("%s upgrade failed: %v", simulationWSLogPrefix, err)
		return
	}

	subID, ch := h.Sim.Subscribe()
	defer func() {
		h.Sim.Unsubscribe(subID)
		conn.Close()
	}()

	go transportweb.DrainUntilClosed(conn)

	for snapshots := range ch {
		payload := simulationSnapshotMessage{
			Type: "snapshot",
			Cars: make([]simulationCarMessage, 0, len(snapshots)),
		}
		for _, car := range snapshots {
			payload.Cars = append(payload.Cars, simulationCarMessage{ID: car.ID, Lat: car.Lat, Lon: car.Lon})
		}
		if err := conn.WriteJSON(payload); err != nil {
			return
		}
	}
}

func (h *SimulationHandler) spawnCars(w http.ResponseWriter, r *http.Request) {
	req, ok := transportweb.DecodeJSON[simulationRequest](w, r)
	if !ok {
		return
	}
	if err := req.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	routes := h.RouteCache.GetMany(req.RouteIDs)
	if len(routes) == 0 {
		http.Error(w, "no routes found", http.StatusNotFound)
		return
	}

	created := h.Sim.SpawnRoutes(routes, req.Count, req.MinStepIndex)
	transportweb.WriteJSON(w, http.StatusOK, simulationResponse{Created: created, Active: h.Sim.Count()})
}

func (r simulationRequest) validate() error {
	if len(r.RouteIDs) == 0 {
		return fmt.Errorf("route_ids must not be empty")
	}
	if r.MinStepIndex < 0 {
		return fmt.Errorf("min_step_index must be non-negative")
	}
	return nil
}

func (r simulationRequest) validateRandom() error {
	if r.MinStepIndex < 0 {
		return fmt.Errorf("min_step_index must be non-negative")
	}
	return nil
}
