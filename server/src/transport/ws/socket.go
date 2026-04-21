package transportws

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

func DrainUntilClosed(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			conn.Close()
			return
		}
	}
}
