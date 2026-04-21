package apiws

import (
	"net/http"

	"github.com/gorilla/websocket"
)

func NewUpgrader(originAllowed func(string) bool) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return originAllowed(r.Header.Get("Origin"))
		},
	}
}
