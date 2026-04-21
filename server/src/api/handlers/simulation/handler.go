package simulationhandler

import (
	"fmt"
	"log"
	"net/http"

	apihttp "nav-system/src/api/http"
	routehandler "nav-system/src/api/handlers/route"
	apiws "nav-system/src/api/ws"
	"nav-system/src/simulation"
)

const (
	simulationWSLogPrefix = "api: simulation websocket"
)

type Handler struct {
	Sim           *simulation.Manager
	RouteCache    *routehandler.Cache
	OriginAllowed func(string) bool
}

type request struct {
	Count        int      `json:"count"`
	RouteIDs     []string `json:"route_ids"`
	MinStepIndex int      `json:"min_step_index"`
}

type response struct {
	Created int `json:"created"`
	Active  int `json:"active"`
}

type snapshotMessage struct {
	Type string       `json:"type"`
	Cars []carMessage `json:"cars"`
}

type carMessage struct {
	ID  string  `json:"id"`
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (h *Handler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.spawnCars(w, r)
	case http.MethodDelete:
		h.Sim.Clear()
		w.WriteHeader(http.StatusNoContent)
	default:
		apihttp.MethodNotAllowed(w)
	}
}

func (h *Handler) HandleRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apihttp.MethodNotAllowed(w)
		return
	}

	req, ok := apihttp.DecodeJSON[request](w, r)
	if !ok {
		return
	}
	if err := req.validateRandom(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created := h.Sim.SpawnRandom(req.Count, req.MinStepIndex)
	apihttp.WriteJSON(w, http.StatusOK, response{Created: created, Active: h.Sim.Count()})
}

func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	upgrader := apiws.NewUpgrader(h.OriginAllowed)
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

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				conn.Close()
				return
			}
		}
	}()

	for snapshots := range ch {
		payload := snapshotMessage{
			Type: "snapshot",
			Cars: make([]carMessage, 0, len(snapshots)),
		}
		for _, car := range snapshots {
			payload.Cars = append(payload.Cars, carMessage{ID: car.ID, Lat: car.Lat, Lon: car.Lon})
		}
		if err := conn.WriteJSON(payload); err != nil {
			return
		}
	}
}

func (h *Handler) spawnCars(w http.ResponseWriter, r *http.Request) {
	req, ok := apihttp.DecodeJSON[request](w, r)
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
	apihttp.WriteJSON(w, http.StatusOK, response{Created: created, Active: h.Sim.Count()})
}

func (r request) validate() error {
	if len(r.RouteIDs) == 0 {
		return fmt.Errorf("route_ids must not be empty")
	}
	if r.MinStepIndex < 0 {
		return fmt.Errorf("min_step_index must be non-negative")
	}
	return nil
}

func (r request) validateRandom() error {
	if r.MinStepIndex < 0 {
		return fmt.Errorf("min_step_index must be non-negative")
	}
	return nil
}
