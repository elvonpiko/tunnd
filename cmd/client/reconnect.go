// Reconnect-related helpers for the tunnd client: registration-rejection
// classification, server-close classification, and the user-facing hint
// printer. Kept separate from main.go so the policy is unit-testable.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/gorilla/websocket"

	"github.com/elvonpiko/tunnd/pkg/proto"
)

// maxRegisterRetries bounds how long a retryable registration rejection is
// retried before giving up. Stale server-side sessions are reaped within
// ~90s (pongWait + ping period); 10 exponential-backoff attempts span
// roughly 3 minutes, comfortably covering that window while not keeping a
// genuinely-conflicted tunnel spinning forever.
const maxRegisterRetries = 10

// retryableError marks a registration rejection that is expected to resolve
// on its own within a bounded window — typically a stale server-side session
// still holding the subdomain or a limit slot during a reconnect.
type retryableError struct {
	code  string
	cause error
}

func (e *retryableError) Error() string { return e.cause.Error() }
func (e *retryableError) Unwrap() error { return e.cause }

// registerRejectionIsRetryable reports whether a register rejection should be
// retried with backoff instead of exiting:
//
//   - subdomain_in_use: with server-side takeover this only fires when a
//     DIFFERENT token holds the name — permanent for us. It still pays to
//     retry briefly in case the conflicting session is a stale one of ours
//     racing the reap.
//   - tunnel_limit_reached: a reconnect can transiently exceed the limit
//     because the server still counts our dead session until its WebSocket
//     times out (~90s). Retrying bridges that window.
//
// Anything else (invalid token, invalid subdomain, …) is fatal.
func registerRejectionIsRetryable(code string) bool {
	switch code {
	case "subdomain_in_use", "tunnel_limit_reached":
		return true
	}
	return false
}

// printRegisterRejection prints the user-facing rejection and an actionable
// hint for each known error code.
func printRegisterRejection(code, message string, localPort int) {
	fmt.Fprintf(os.Stderr, "\n  ✗  %s\n", message)
	switch code {
	case "subdomain_in_use":
		fmt.Fprintf(os.Stderr, "     Try a different subdomain: tunnd http %d --subdomain myapp2\n", localPort)
	case "tunnel_limit_reached":
		fmt.Fprintln(os.Stderr, "     Close another tunnel using this token, or ask your admin to raise the limit.")
	case "handshake_failed":
		fmt.Fprintln(os.Stderr, "     Your token may be invalid or revoked.")
		fmt.Fprintln(os.Stderr, "     Run: tunnd setup   to reconfigure.")
	}
	fmt.Fprintln(os.Stderr)
}

// classifyClosedByServer inspects a terminal WebSocket read error. When the
// server closed the session deliberately (close code 4429 — the tunnel was
// taken over by a newer client or the token was revoked), it returns
// fatal=true plus the server's reason text. The caller must NOT reconnect in
// that case: a reconnect loop between two clients fighting over one
// subdomain is exactly what this code exists to prevent.
func classifyClosedByServer(err error) (fatal bool, reason string) {
	var ce *websocket.CloseError
	if errors.As(err, &ce) && ce.Code == proto.CloseCodeSessionTakenOver {
		return true, ce.Text
	}
	return false, ""
}
