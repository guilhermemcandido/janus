package vtcodec

import (
	"testing"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"google.golang.org/grpc/mem"
)

func TestCodec_MarshalUnmarshalRoundTrip(t *testing.T) {
	req := &pb.SubmitOrderRequest{Symbol: "AAPL", Side: pb.Side_BUY, Price: 100, Quantity: 5}

	data, err := (codec{}).Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	got := &pb.SubmitOrderRequest{}
	if err := (codec{}).Unmarshal(data, got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Symbol != req.Symbol || got.Price != req.Price || got.Quantity != req.Quantity {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, req)
	}
}

func TestCodec_UnmarshalAcrossMultipleBuffers(t *testing.T) {
	req := &pb.SubmitOrderRequest{Symbol: "MSFT", Side: pb.Side_SELL, Price: 250, Quantity: 9}
	data, err := (codec{}).Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	whole := data.Materialize()
	mid := len(whole) / 2
	split := mem.BufferSlice{mem.SliceBuffer(whole[:mid]), mem.SliceBuffer(whole[mid:])}

	got := &pb.SubmitOrderRequest{}
	if err := (codec{}).Unmarshal(split, got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Symbol != req.Symbol || got.Price != req.Price || got.Quantity != req.Quantity {
		t.Fatalf("Unmarshal() = %+v, want %+v", got, req)
	}
}

func TestCodec_MarshalRejectsNonVtprotoMessage(t *testing.T) {
	if _, err := (codec{}).Marshal("not a proto message"); err == nil {
		t.Fatalf("Marshal(string) = nil error, want an error")
	}
}

func TestCodec_UnmarshalRejectsNonVtprotoMessage(t *testing.T) {
	var s string
	if err := (codec{}).Unmarshal(mem.BufferSlice{mem.SliceBuffer("x")}, &s); err == nil {
		t.Fatalf("Unmarshal(*string) = nil error, want an error")
	}
}

func TestCodec_Name(t *testing.T) {
	if got := (codec{}).Name(); got != "proto" {
		t.Fatalf("Name() = %q, want %q", got, "proto")
	}
}

func TestRegister_SafeForConcurrentCalls(t *testing.T) {
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			Register()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
