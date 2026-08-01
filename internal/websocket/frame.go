package websocket

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Opcode identifies a frame's payload type, per RFC 6455 section 11.8.
type Opcode uint8

const (
	OpContinuation Opcode = 0x0
	OpText         Opcode = 0x1
	OpBinary       Opcode = 0x2
	OpClose        Opcode = 0x8
	OpPing         Opcode = 0x9
	OpPong         Opcode = 0xA
)

func (op Opcode) isControl() bool {
	return op >= OpClose
}

type frameHeader struct {
	fin     bool
	opcode  Opcode
	masked  bool
	length  uint64
	maskKey [4]byte
}

// readFrameHeader parses one RFC 6455 frame header from r, leaving the payload unread.
func readFrameHeader(r io.Reader) (frameHeader, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return frameHeader{}, err
	}

	if b[0]&0x70 != 0 {
		return frameHeader{}, fmt.Errorf("websocket: nonzero RSV bits (no extension negotiated)")
	}

	h := frameHeader{
		fin:    b[0]&0x80 != 0,
		opcode: Opcode(b[0] & 0x0F),
		masked: b[1]&0x80 != 0,
		length: uint64(b[1] & 0x7F),
	}

	switch h.length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return frameHeader{}, err
		}
		h.length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return frameHeader{}, err
		}
		h.length = binary.BigEndian.Uint64(ext[:])
	}

	if h.opcode.isControl() && (!h.fin || h.length > 125) {
		return frameHeader{}, fmt.Errorf("websocket: control frame must be unfragmented and <=125 bytes")
	}

	if h.masked {
		if _, err := io.ReadFull(r, h.maskKey[:]); err != nil {
			return frameHeader{}, err
		}
	}

	return h, nil
}

// writeFrame writes a single, unfragmented, unmasked frame - the only kind a server ever sends.
func writeFrame(w io.Writer, op Opcode, payload []byte) error {
	b0 := byte(op) | 0x80 // FIN always set: this package never fragments outgoing frames

	var header []byte
	n := len(payload)
	switch {
	case n <= 125:
		header = []byte{b0, byte(n)}
	case n <= 65535:
		header = make([]byte, 4)
		header[0], header[1] = b0, 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
	default:
		header = make([]byte, 10)
		header[0], header[1] = b0, 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
	}

	// One combined write, not header then payload separately: a zero-length payload (e.g. an
	// empty Close frame) makes that second write a no-op on most Writers, but net.Pipe blocks on it forever.
	_, err := w.Write(append(header, payload...))
	return err
}

// maskBytes applies the RFC 6455 masking algorithm in place; it's its own inverse.
func maskBytes(key [4]byte, data []byte) {
	for i := range data {
		data[i] ^= key[i%4]
	}
}
