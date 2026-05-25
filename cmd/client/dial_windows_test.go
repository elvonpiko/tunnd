//go:build windows

package main

import (
	"net"
	"testing"
	"time"
)

// TestIsConnectionRefused_Windows produces a real connectex error by
// dialing a closed port on the local Windows runner. Asserts the helper
// recognizes the platform-specific "actively refused" message.
func TestIsConnectionRefused_Windows(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected error dialing closed port %s, got nil", addr)
	}
	if !isConnectionRefused(err) {
		t.Fatalf("isConnectionRefused(%v) = false, want true", err)
	}
}
