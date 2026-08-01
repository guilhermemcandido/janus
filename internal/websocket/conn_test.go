package websocket

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// rawClient writes/reads raw frame bytes on the other end of a net.Pipe, standing in for a browser.
type rawClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
}

func newTestPair(t *testing.T) (*Conn, *rawClient) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() { serverSide.Close(); clientSide.Close() })
	return newConn(serverSide, bufio.NewReader(serverSide)), &rawClient{t: t, conn: clientSide, br: bufio.NewReader(clientSide)}
}

// send writes a single masked frame, as a real browser's client implementation always must.
func (c *rawClient) send(op Opcode, payload []byte) {
	c.t.Helper()
	key := [4]byte{0x01, 0x02, 0x03, 0x04}
	masked := append([]byte(nil), payload...)
	maskBytes(key, masked)

	b0 := 0x80 | byte(op)
	var header []byte
	switch {
	case len(masked) <= 125:
		header = []byte{b0, 0x80 | byte(len(masked))}
	default:
		header = []byte{b0, 0x80 | 126}
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(len(masked)))
		header = append(header, ext[:]...)
	}
	header = append(header, key[:]...)
	if _, err := c.conn.Write(append(header, masked...)); err != nil {
		c.t.Fatalf("write raw frame: %v", err)
	}
}

// sendFragment writes one frame of a fragmented message without the FIN bit set.
func (c *rawClient) sendFragment(op Opcode, payload []byte, fin bool) {
	c.t.Helper()
	key := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}
	masked := append([]byte(nil), payload...)
	maskBytes(key, masked)

	b0 := byte(op)
	if fin {
		b0 |= 0x80
	}
	header := append([]byte{b0, 0x80 | byte(len(masked))}, key[:]...)
	if _, err := c.conn.Write(append(header, masked...)); err != nil {
		c.t.Fatalf("write raw fragment: %v", err)
	}
}

func (c *rawClient) readFrame() (frameHeader, []byte) {
	c.t.Helper()
	hdr, err := readFrameHeader(c.br)
	if err != nil {
		c.t.Fatalf("readFrameHeader: %v", err)
	}
	payload := make([]byte, hdr.length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		c.t.Fatalf("read payload: %v", err)
	}
	return hdr, payload
}

func TestConn_ReadMessageDecodesTextFrame(t *testing.T) {
	conn, client := newTestPair(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.send(OpText, []byte("hello"))
	}()

	op, payload, err := conn.ReadMessage()
	<-done
	if err != nil {
		t.Fatalf("ReadMessage returned error: %v", err)
	}
	if op != OpText || string(payload) != "hello" {
		t.Fatalf("got (%v, %q), want (OpText, \"hello\")", op, payload)
	}
}

func TestConn_ReadMessageReassemblesFragmentedMessage(t *testing.T) {
	conn, client := newTestPair(t)

	go func() {
		client.sendFragment(OpText, []byte("hello "), false)
		client.sendFragment(OpContinuation, []byte("world"), true)
	}()

	op, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage returned error: %v", err)
	}
	if op != OpText || string(payload) != "hello world" {
		t.Fatalf("got (%v, %q), want (OpText, \"hello world\")", op, payload)
	}
}

func TestConn_ReadMessageAnswersPingWithPong(t *testing.T) {
	conn, client := newTestPair(t)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		client.send(OpPing, []byte("ping-payload"))
		hdr, payload := client.readFrame()
		if hdr.opcode != OpPong || string(payload) != "ping-payload" {
			t.Errorf("got (%v, %q), want (OpPong, \"ping-payload\")", hdr.opcode, payload)
		}
		client.send(OpText, []byte("after ping"))
	}()

	op, payload, err := conn.ReadMessage()
	wg.Wait()
	if err != nil {
		t.Fatalf("ReadMessage returned error: %v", err)
	}
	if op != OpText || string(payload) != "after ping" {
		t.Fatalf("got (%v, %q), want (OpText, \"after ping\")", op, payload)
	}
}

func TestConn_ReadMessageRejectsUnmaskedFrame(t *testing.T) {
	conn, client := newTestPair(t)

	go func() {
		// An unmasked frame straight from the wire, as a spec-violating client might send.
		client.conn.Write([]byte{0x81, 0x03, 'a', 'b', 'c'})
		client.readFrame() // drain the close frame the server sends back, or its write blocks forever
	}()

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatalf("ReadMessage returned nil error, want a rejection of an unmasked client frame")
	}
}

func TestConn_ReadMessageReturnsErrClosedOnPeerClose(t *testing.T) {
	conn, client := newTestPair(t)

	go func() {
		client.send(OpClose, nil)
		client.readFrame() // drain the echoed close frame, or the server's write blocks forever
	}()

	op, _, err := conn.ReadMessage()
	if op != OpClose || err != ErrClosed {
		t.Fatalf("got (%v, %v), want (OpClose, ErrClosed)", op, err)
	}
}

func TestConn_CloseSendsCloseFrame(t *testing.T) {
	conn, client := newTestPair(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		hdr, payload := client.readFrame()
		if hdr.opcode != OpClose {
			t.Errorf("opcode = %v, want OpClose", hdr.opcode)
		}
		if len(payload) < 2 || binary.BigEndian.Uint16(payload) != closeNormal {
			t.Errorf("close payload = % x, want status code %d", payload, closeNormal)
		}
	}()

	if err := conn.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the close frame")
	}
}

func TestConn_WriteMessageIsConcurrencySafe(t *testing.T) {
	conn, client := newTestPair(t)

	const writers = 10
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := conn.WriteMessage(OpText, bytes.Repeat([]byte{byte('a' + i)}, 5)); err != nil {
				t.Errorf("WriteMessage returned error: %v", err)
			}
		}(i)
	}

	readDone := make(chan int, 1)
	go func() {
		count := 0
		for count < writers {
			client.readFrame()
			count++
		}
		readDone <- count
	}()

	wg.Wait()
	select {
	case count := <-readDone:
		if count != writers {
			t.Fatalf("received %d frames, want %d", count, writers)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting to receive all frames (a corrupted/interleaved frame likely broke parsing)")
	}
}
