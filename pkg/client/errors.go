package client

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FriendlyError returns a short, human-readable message for common connectivity errors, or err's own message otherwise.
func FriendlyError(err error) string {
	if status.Code(err) == codes.Unavailable {
		return "server unreachable"
	}
	return err.Error()
}
