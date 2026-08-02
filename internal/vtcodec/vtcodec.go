// Package vtcodec registers a gRPC CodecV2 calling vtprotobuf's MarshalVT/UnmarshalVT directly -
// vtprotobuf's own codec/grpc only implements the older Codec interface, which this grpc-go bridges.
package vtcodec

import (
	"fmt"
	"sync"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
)

type vtMessage interface {
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

type codec struct{}

func (codec) Name() string { return "proto" }

func (codec) Marshal(v any) (mem.BufferSlice, error) {
	vt, ok := v.(vtMessage)
	if !ok {
		return nil, fmt.Errorf("vtcodec: %T does not implement vtprotobuf helpers", v)
	}
	data, err := vt.MarshalVT()
	if err != nil {
		return nil, err
	}
	return mem.BufferSlice{mem.SliceBuffer(data)}, nil
}

// Unmarshal takes the single-buffer fast path directly off data - safe because vtprotobuf's
// "unmarshal" feature (not "unmarshal_unsafe") always copies out what it needs before returning.
func (codec) Unmarshal(data mem.BufferSlice, v any) error {
	vt, ok := v.(vtMessage)
	if !ok {
		return fmt.Errorf("vtcodec: %T does not implement vtprotobuf helpers", v)
	}
	if len(data) == 1 {
		return vt.UnmarshalVT(data[0].ReadOnlyData())
	}
	return vt.UnmarshalVT(data.Materialize())
}

var registerOnce sync.Once

// Register installs this codec, overriding grpc's default. Called explicitly (from Dial/NewServer,
// possibly concurrently) rather than from init - RegisterCodecV2 itself isn't safe for that.
func Register() {
	registerOnce.Do(func() { encoding.RegisterCodecV2(codec{}) })
}
