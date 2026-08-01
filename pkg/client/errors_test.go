package client

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFriendlyError_UnavailableGetsShortMessage(t *testing.T) {
	err := status.Error(codes.Unavailable, "connection error: desc = \"transport: Error while dialing...\"")

	if got := FriendlyError(err); got != "server unreachable" {
		t.Fatalf("FriendlyError() = %q, want %q", got, "server unreachable")
	}
}

func TestFriendlyError_OtherErrorsPassThrough(t *testing.T) {
	err := errors.New("some other error")

	if got := FriendlyError(err); got != "some other error" {
		t.Fatalf("FriendlyError() = %q, want %q", got, "some other error")
	}
}
