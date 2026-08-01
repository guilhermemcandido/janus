package websocket

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// maxMessageSize bounds a single message, guarding against a bogus or malicious length field.
const maxMessageSize = 1 << 20 // 1 MiB

const (
	closeNormal        uint16 = 1000
	closeProtocolError uint16 = 1002
	closeMessageTooBig uint16 = 1009
)

// ErrClosed is returned by ReadMessage once the connection has completed the close handshake.
var ErrClosed = errors.New("websocket: connection closed")

// Conn is an upgraded WebSocket connection. Safe for one reader and multiple concurrent writers.
type Conn struct {
	conn net.Conn
	br   *bufio.Reader

	writeMu sync.Mutex
	closeMu sync.Mutex
	closed  bool
}

func newConn(conn net.Conn, br *bufio.Reader) *Conn {
	return &Conn{conn: conn, br: br}
}

// ReadMessage blocks for the next text or binary message, transparently answering pings and
// reassembling fragmented messages. It returns ErrClosed once the peer closes the connection.
func (c *Conn) ReadMessage() (Opcode, []byte, error) {
	for {
		hdr, err := readFrameHeader(c.br)
		if err != nil {
			return 0, nil, err
		}
		if !hdr.masked {
			c.sendClose(closeProtocolError, "client frames must be masked")
			return 0, nil, fmt.Errorf("websocket: received unmasked frame from client")
		}
		if hdr.length > maxMessageSize {
			c.sendClose(closeMessageTooBig, "message too large")
			return 0, nil, fmt.Errorf("websocket: message of %d bytes exceeds the %d byte limit", hdr.length, maxMessageSize)
		}

		payload := make([]byte, hdr.length)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return 0, nil, err
		}
		maskBytes(hdr.maskKey, payload)

		switch hdr.opcode {
		case OpPing:
			if err := c.writeControl(OpPong, payload); err != nil {
				return 0, nil, err
			}
			continue
		case OpPong:
			continue
		case OpClose:
			_ = c.writeControl(OpClose, payload)
			c.conn.Close()
			return OpClose, payload, ErrClosed
		case OpContinuation:
			c.sendClose(closeProtocolError, "unexpected continuation frame")
			return 0, nil, fmt.Errorf("websocket: unexpected continuation frame")
		case OpText, OpBinary:
			if hdr.fin {
				return hdr.opcode, payload, nil
			}
			full, err := c.readContinuations(payload)
			if err != nil {
				return 0, nil, err
			}
			return hdr.opcode, full, nil
		default:
			c.sendClose(closeProtocolError, "unknown opcode")
			return 0, nil, fmt.Errorf("websocket: unknown opcode %#x", hdr.opcode)
		}
	}
}

// readContinuations accumulates continuation frames until FIN, appending onto an already-read first fragment.
func (c *Conn) readContinuations(first []byte) ([]byte, error) {
	full := first
	for {
		hdr, err := readFrameHeader(c.br)
		if err != nil {
			return nil, err
		}
		if hdr.opcode != OpContinuation {
			c.sendClose(closeProtocolError, "expected a continuation frame")
			return nil, fmt.Errorf("websocket: expected continuation frame, got opcode %#x", hdr.opcode)
		}
		if !hdr.masked {
			c.sendClose(closeProtocolError, "client frames must be masked")
			return nil, fmt.Errorf("websocket: received unmasked frame from client")
		}
		if uint64(len(full))+hdr.length > maxMessageSize {
			c.sendClose(closeMessageTooBig, "message too large")
			return nil, fmt.Errorf("websocket: fragmented message exceeds the %d byte limit", maxMessageSize)
		}

		chunk := make([]byte, hdr.length)
		if _, err := io.ReadFull(c.br, chunk); err != nil {
			return nil, err
		}
		maskBytes(hdr.maskKey, chunk)
		full = append(full, chunk...)

		if hdr.fin {
			return full, nil
		}
	}
}

// WriteMessage sends payload as a single unfragmented text or binary frame.
func (c *Conn) WriteMessage(op Opcode, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(c.conn, op, payload)
}

func (c *Conn) writeControl(op Opcode, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(c.conn, op, payload)
}

// sendClose sends a close frame with the given status code and reason, per RFC 6455 section 7.1.7.
func (c *Conn) sendClose(code uint16, reason string) {
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload, code)
	copy(payload[2:], reason)
	_ = c.writeControl(OpClose, payload)
}

// Close sends a normal close frame (if the connection isn't already closed) and closes the socket.
func (c *Conn) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	c.sendClose(closeNormal, "")
	return c.conn.Close()
}
