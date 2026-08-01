package websocket

import (
	"bytes"
	"testing"
)

func TestWriteFrame_SmallPayload(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, OpText, []byte("abc")); err != nil {
		t.Fatalf("writeFrame returned error: %v", err)
	}

	want := []byte{0x81, 0x03, 'a', 'b', 'c'} // FIN|text, unmasked len 3
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("frame = % x, want % x", buf.Bytes(), want)
	}
}

func TestWriteFrame_MediumPayloadUses16BitLength(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, 200)
	var buf bytes.Buffer
	if err := writeFrame(&buf, OpBinary, payload); err != nil {
		t.Fatalf("writeFrame returned error: %v", err)
	}

	wantHeader := []byte{0x82, 126, 0x00, 0xC8} // FIN|binary, 126 marker, len 200 big-endian
	if !bytes.Equal(buf.Bytes()[:4], wantHeader) {
		t.Fatalf("header = % x, want % x", buf.Bytes()[:4], wantHeader)
	}
	if !bytes.Equal(buf.Bytes()[4:], payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestWriteFrame_LargePayloadUses64BitLength(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, 70000)
	var buf bytes.Buffer
	if err := writeFrame(&buf, OpBinary, payload); err != nil {
		t.Fatalf("writeFrame returned error: %v", err)
	}

	wantHeader := []byte{0x82, 127, 0, 0, 0, 0, 0, 1, 0x11, 0x70} // 70000 = 0x11170
	if !bytes.Equal(buf.Bytes()[:10], wantHeader) {
		t.Fatalf("header = % x, want % x", buf.Bytes()[:10], wantHeader)
	}
}

func TestReadFrameHeader_MaskedSmallFrame(t *testing.T) {
	maskKey := [4]byte{0x01, 0x02, 0x03, 0x04}
	raw := append([]byte{0x81, 0x85}, maskKey[:]...) // FIN|text, masked, len 5
	hdr, err := readFrameHeader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readFrameHeader returned error: %v", err)
	}
	if !hdr.fin || hdr.opcode != OpText || !hdr.masked || hdr.length != 5 || hdr.maskKey != maskKey {
		t.Fatalf("hdr = %+v, want fin=true opcode=text masked=true length=5 maskKey=%v", hdr, maskKey)
	}
}

func TestReadFrameHeader_Uses16BitLength(t *testing.T) {
	raw := []byte{0x81, 126, 0x00, 0xC8} // unmasked, len 200
	hdr, err := readFrameHeader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readFrameHeader returned error: %v", err)
	}
	if hdr.length != 200 {
		t.Fatalf("length = %d, want 200", hdr.length)
	}
}

func TestReadFrameHeader_Uses64BitLength(t *testing.T) {
	raw := []byte{0x81, 127, 0, 0, 0, 0, 0, 1, 0x11, 0x70} // unmasked, len 70000
	hdr, err := readFrameHeader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readFrameHeader returned error: %v", err)
	}
	if hdr.length != 70000 {
		t.Fatalf("length = %d, want 70000", hdr.length)
	}
}

func TestReadFrameHeader_RejectsNonzeroRSVBits(t *testing.T) {
	raw := []byte{0x80 | 0x40 | byte(OpText), 0x00} // RSV1 set alongside FIN|text
	if _, err := readFrameHeader(bytes.NewReader(raw)); err == nil {
		t.Fatalf("readFrameHeader returned nil error, want a rejection of nonzero RSV bits")
	}
}

func TestReadFrameHeader_RejectsFragmentedControlFrame(t *testing.T) {
	raw := []byte{byte(OpClose), 0x00} // FIN not set on a control frame
	if _, err := readFrameHeader(bytes.NewReader(raw)); err == nil {
		t.Fatalf("readFrameHeader returned nil error, want a rejection of a fragmented control frame")
	}
}

func TestReadFrameHeader_RejectsOversizedControlFrame(t *testing.T) {
	raw := []byte{0x80 | byte(OpPing), 126, 0x00, 0xC8} // FIN|ping claiming an extended length of 200
	if _, err := readFrameHeader(bytes.NewReader(raw)); err == nil {
		t.Fatalf("readFrameHeader returned nil error, want a rejection of an oversized control frame")
	}
}

func TestMaskBytes_IsSelfInverse(t *testing.T) {
	key := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	original := []byte("the quick brown fox jumps over the lazy dog")
	data := append([]byte(nil), original...)

	maskBytes(key, data)
	if bytes.Equal(data, original) {
		t.Fatalf("masking left data unchanged")
	}
	maskBytes(key, data)
	if !bytes.Equal(data, original) {
		t.Fatalf("masking twice with the same key = %q, want original %q", data, original)
	}
}
