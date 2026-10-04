package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/elvonpiko/tunnd/pkg/proto"
)

func TestRegisterRejectionIsRetryable(t *testing.T) {
	for _, code := range []string{"subdomain_in_use", "tunnel_limit_reached"} {
		if !registerRejectionIsRetryable(code) {
			t.Errorf("code %q should be retryable", code)
		}
	}
	for _, code := range []string{
		"handshake_failed", "invalid_token", "invalid_subdomain",
		"tcp_port_unavailable", "", "unknown_code",
	} {
		if registerRejectionIsRetryable(code) {
			t.Errorf("code %q should NOT be retryable", code)
		}
	}
}

func TestClassifyClosedByServer(t *testing.T) {
	takenOver := &websocket.CloseError{
		Code: proto.CloseCodeSessionTakenOver,
		Text: "subdomain re-registered by a newer session of the same token",
	}

	fatal, reason := classifyClosedByServer(takenOver)
	if !fatal {
		t.Fatal("close 4429 must be classified as fatal (no auto-reconnect)")
	}
	if reason != takenOver.Text {
		t.Errorf("reason = %q, want %q", reason, takenOver.Text)
	}

	// Wrapped chains (errors.As semantics) must still be detected.
	wrapped := fmt.Errorf("ws read: %w", takenOver)
	if fatal, _ := classifyClosedByServer(wrapped); !fatal {
		t.Fatal("wrapped close 4429 must be classified as fatal")
	}

	// Normal closures and I/O errors must NOT be fatal — the client
	// reconnects for those.
	for _, err := range []error{
		&websocket.CloseError{Code: websocket.CloseNormalClosure, Text: ""},
		&websocket.CloseError{Code: websocket.CloseGoingAway, Text: ""},
		errors.New("connection reset by peer"),
		nil,
	} {
		if fatal, _ := classifyClosedByServer(err); fatal {
			t.Errorf("error %v must NOT be classified as fatal", err)
		}
	}
}

func TestRetryableErrorCarriesCode(t *testing.T) {
	cause := errors.New("boom")
	re := &retryableError{code: "subdomain_in_use", cause: cause}

	if re.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", re.Error(), "boom")
	}
	if re.Unwrap() != cause {
		t.Error("Unwrap must return the wrapped cause")
	}
	if !registerRejectionIsRetryable(re.code) {
		t.Error("carried code must classify as retryable")
	}
}
