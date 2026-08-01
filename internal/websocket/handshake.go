package websocket

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// magicGUID is fixed by RFC 6455 section 1.3 and appended to the client's key before hashing.
const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// acceptKey computes the Sec-WebSocket-Accept value for a given Sec-WebSocket-Key.
func acceptKey(clientKey string) string {
	sum := sha1.Sum([]byte(clientKey + magicGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContainsToken(h, token string) bool {
	for _, part := range strings.Split(h, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// Upgrade completes the RFC 6455 handshake on r's connection and takes it over via http.Hijacker.
// The caller must not use w or r again after Upgrade returns successfully.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet {
		http.Error(w, "websocket: expected GET", http.StatusMethodNotAllowed)
		return nil, fmt.Errorf("websocket: expected GET, got %s", r.Method)
	}
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket: expected an Upgrade request", http.StatusBadRequest)
		return nil, fmt.Errorf("websocket: missing or invalid Upgrade/Connection headers")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "websocket: unsupported version", http.StatusUpgradeRequired)
		return nil, fmt.Errorf("websocket: unsupported Sec-WebSocket-Version %q", r.Header.Get("Sec-WebSocket-Version"))
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "websocket: missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, fmt.Errorf("websocket: missing Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket: server does not support hijacking", http.StatusInternalServerError)
		return nil, fmt.Errorf("websocket: ResponseWriter does not implement http.Hijacker")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket: hijack: %w", err)
	}

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := rw.WriteString(response); err != nil {
		conn.Close()
		return nil, fmt.Errorf("websocket: write handshake response: %w", err)
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("websocket: flush handshake response: %w", err)
	}

	return newConn(conn, rw.Reader), nil
}
