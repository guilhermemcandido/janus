package web

import (
	"log"
	"net/http"

	"github.com/guilhermemcandido/janus/internal/websocket"
	"github.com/guilhermemcandido/janus/pkg/client"
)

// NewWebSocketHandler returns an http.Handler that upgrades every request to a WebSocket connection bridged to c.
func NewWebSocketHandler(c *client.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r)
		if err != nil {
			log.Printf("web: upgrade failed: %v", err)
			return
		}
		HandleConn(conn, c)
	})
}
