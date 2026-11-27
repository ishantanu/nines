package observability

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

// Error classes never carry the error text.
func TestErrorClassIsCoarseAndSecretFree(t *testing.T) {
	for want, err := range map[string]error{
		"none":     nil,
		"timeout":  fmt.Errorf("open tenant: %w", context.DeadlineExceeded),
		"canceled": context.Canceled,
		"network":  &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		"other":    errors.New("password=synthetic-secret rejected"),
	} {
		if got := ErrorClass(err); got != want {
			t.Errorf("%v: got %s, want %s", err, got, want)
		}
	}
}
