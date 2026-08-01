package websocket

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAcceptKey_MatchesRFC6455Example(t *testing.T) {
	// The worked example from RFC 6455 section 1.3.
	got := acceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("acceptKey() = %q, want %q", got, want)
	}
}

func TestHeaderContainsToken(t *testing.T) {
	tests := []struct {
		header string
		token  string
		want   bool
	}{
		{"Upgrade", "upgrade", true},
		{"keep-alive, Upgrade", "upgrade", true},
		{"Upgrade, keep-alive", "upgrade", true},
		{"keep-alive", "upgrade", false},
		{"", "upgrade", false},
	}
	for _, tt := range tests {
		if got := headerContainsToken(tt.header, tt.token); got != tt.want {
			t.Errorf("headerContainsToken(%q, %q) = %v, want %v", tt.header, tt.token, got, tt.want)
		}
	}
}

func TestUpgrade_RejectsNonGET(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/ws", nil)
	w := httptest.NewRecorder()
	if _, err := Upgrade(w, r); err == nil {
		t.Fatalf("Upgrade returned nil error for a non-GET request")
	}
}

func TestUpgrade_RejectsMissingUpgradeHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-WebSocket-Version", "13")
	r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	w := httptest.NewRecorder()
	if _, err := Upgrade(w, r); err == nil {
		t.Fatalf("Upgrade returned nil error for a request missing the Upgrade header")
	}
}

func TestUpgrade_RejectsWrongVersion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Sec-WebSocket-Version", "8")
	r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	w := httptest.NewRecorder()
	if _, err := Upgrade(w, r); err == nil {
		t.Fatalf("Upgrade returned nil error for an unsupported Sec-WebSocket-Version")
	}
}

func TestUpgrade_RejectsMissingKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Sec-WebSocket-Version", "13")
	w := httptest.NewRecorder()
	if _, err := Upgrade(w, r); err == nil {
		t.Fatalf("Upgrade returned nil error for a request missing Sec-WebSocket-Key")
	}
}

// TestUpgrade_FullHandshakeOverRealHTTPServer drives the handshake and a message exchange over a
// real TCP connection, with a hand-written raw client - there's no client half of this package.
func TestUpgrade_FullHandshakeOverRealHTTPServer(t *testing.T) {
	serverErrs := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			serverErrs <- err
			return
		}
		defer conn.Close()

		op, payload, err := conn.ReadMessage()
		if err != nil {
			serverErrs <- err
			return
		}
		if err := conn.WriteMessage(op, append([]byte("echo: "), payload...)); err != nil {
			serverErrs <- err
			return
		}
		serverErrs <- nil
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer conn.Close()

	clientKey := "dGhlIHNhbXBsZSBub25jZQ=="
	request := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", addr, clientKey)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write handshake request: %v", err)
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	if !bytes.Contains([]byte(statusLine), []byte("101")) {
		t.Fatalf("status line = %q, want a 101 Switching Protocols response", statusLine)
	}

	var acceptHeader string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read header line: %v", err)
		}
		if line == "\r\n" {
			break
		}
		if bytes.HasPrefix([]byte(line), []byte("Sec-WebSocket-Accept:")) {
			acceptHeader = line
		}
	}
	if want := acceptKey(clientKey); !bytes.Contains([]byte(acceptHeader), []byte(want)) {
		t.Fatalf("Sec-WebSocket-Accept header = %q, want it to contain %q", acceptHeader, want)
	}

	// Send a masked text frame, as a real browser client must.
	key := [4]byte{0x11, 0x22, 0x33, 0x44}
	body := []byte("ping")
	masked := append([]byte(nil), body...)
	maskBytes(key, masked)
	frame := append([]byte{0x81, 0x80 | byte(len(masked))}, key[:]...)
	frame = append(frame, masked...)
	if _, err := conn.Write(frame); err != nil {
		t.Fatalf("write data frame: %v", err)
	}

	hdr, err := readFrameHeader(br)
	if err != nil {
		t.Fatalf("readFrameHeader: %v", err)
	}
	payload := make([]byte, hdr.length)
	if _, err := readFullPayload(br, payload); err != nil {
		t.Fatalf("read echoed payload: %v", err)
	}
	if hdr.opcode != OpText || string(payload) != "echo: ping" {
		t.Fatalf("got (%v, %q), want (OpText, \"echo: ping\")", hdr.opcode, payload)
	}

	select {
	case err := <-serverErrs:
		if err != nil {
			t.Fatalf("server handler returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the server handler to finish")
	}
}

func readFullPayload(r *bufio.Reader, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		m, err := r.Read(buf[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
