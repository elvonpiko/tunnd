package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestDialLocal_IPv4Only binds a listener on 127.0.0.1 only and asserts
// dialLocal reaches it within 100 ms. Validates the IPv4 leg of P1.
func TestDialLocal_IPv4Only(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on 127.0.0.1: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port

	start := time.Now()
	conn, err := dialLocal(port, 1*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("dialLocal(%d): %v", port, err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if elapsed > 100*time.Millisecond {
		t.Fatalf("dial took %v, want ≤ 100ms", elapsed)
	}
}

// TestDialLocal_IPv6Only binds a listener on [::1] only and asserts
// dialLocal reaches it. Validates the IPv6 leg of P1. Skipped if the
// host has no usable IPv6 loopback or if "localhost" does not resolve
// to ::1 on the test host (some Linux containers ship /etc/hosts with
// only the IPv4 mapping, in which case dialLocal cannot reach an
// IPv6-only listener regardless of correctness).
func TestDialLocal_IPv6Only(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("ipv6 loopback not configured")
	}
	t.Cleanup(func() { _ = ln.Close() })

	addrs, _ := net.LookupHost("localhost")
	hasV6 := false
	for _, a := range addrs {
		if a == "::1" {
			hasV6 = true
			break
		}
	}
	if !hasV6 {
		t.Skip("ipv6 loopback not configured")
	}

	port := ln.Addr().(*net.TCPAddr).Port

	start := time.Now()
	conn, err := dialLocal(port, 1*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("dialLocal(%d): %v", port, err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if elapsed > 100*time.Millisecond {
		t.Fatalf("dial took %v, want ≤ 100ms", elapsed)
	}
}

// TestDialLocal_Both binds listeners on BOTH IPv4 and IPv6 loopback on
// the same numeric port. Happy Eyeballs picks one. Skipped if a port
// usable for both families on this host can't be allocated.
func TestDialLocal_Both(t *testing.T) {
	// Pick an available port via a temporary IPv4 listener.
	tmp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("temp listen: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	ln4, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("could not re-bind ipv4 on chosen port %d: %v", port, err)
	}
	t.Cleanup(func() { _ = ln4.Close() })

	ln6, err := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", port))
	if err != nil {
		t.Skipf("could not bind ipv6 on chosen port %d: %v", port, err)
	}
	t.Cleanup(func() { _ = ln6.Close() })

	conn, err := dialLocal(port, 1*time.Second)
	if err != nil {
		t.Fatalf("dialLocal(%d): %v", port, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

// TestDialLocal_Refused dials a port with no listener and asserts the
// error is recognized as a connection refusal.
func TestDialLocal_Refused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	conn, err := dialLocal(port, 1*time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected error dialing closed port %d, got nil", port)
	}
	if !isConnectionRefused(err) {
		t.Fatalf("isConnectionRefused(%v) = false, want true", err)
	}
}

// TestIsConnectionRefused_POSIX synthesises a wrapped ECONNREFUSED and
// asserts the helper recognizes it via errors.Is.
func TestIsConnectionRefused_POSIX(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
	}
	if !isConnectionRefused(err) {
		t.Fatalf("isConnectionRefused(synthetic ECONNREFUSED) = false, want true")
	}
}

// TestIsConnectionRefused_NilAndUnrelated covers the two negative cases.
func TestIsConnectionRefused_NilAndUnrelated(t *testing.T) {
	if isConnectionRefused(nil) {
		t.Fatal("isConnectionRefused(nil) = true, want false")
	}
	if isConnectionRefused(errors.New("some other error")) {
		t.Fatal("isConnectionRefused(\"some other error\") = true, want false")
	}
}

// TestDialLocal_IPv4_FastPath asserts that on Linux/macOS, dialLocal
// against a 127.0.0.1 listener returns within 5 ms wall-clock under
// cold-loopback conditions. This is the preservation guard for P14
// (Linux/macOS first-attempt latency unchanged after the DualStack
// switch). The listener is held open across all 5 samples; we take
// the max, not the mean, to keep the assertion simple and strict.
//
// Skipped on Windows where loopback dial latency is variable enough
// that a 5 ms ceiling would be flaky in CI. The skip is at runtime
// (rather than a build tag) so the test still compiles on Windows.
func TestDialLocal_IPv4_FastPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("loopback dial latency on Windows is too variable for a 5ms assertion")
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on 127.0.0.1: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port

	const samples = 5
	const ceiling = 5 * time.Millisecond

	timings := make([]time.Duration, samples)
	conns := make([]net.Conn, 0, samples)
	t.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
	})

	for i := 0; i < samples; i++ {
		start := time.Now()
		conn, err := dialLocal(port, 1*time.Second)
		timings[i] = time.Since(start)
		if err != nil {
			t.Fatalf("dialLocal sample %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	var worst time.Duration
	for _, d := range timings {
		if d > worst {
			worst = d
		}
	}
	if worst > ceiling {
		t.Fatalf("dialLocal worst-of-%d = %v, want ≤ %v; timings=%v",
			samples, worst, ceiling, timings)
	}
}

// BenchmarkDialLocal_IPv4 establishes a baseline for the cold-dial
// latency of dialLocal against a 127.0.0.1 listener. It is a
// preservation benchmark for P14 — the new DualStack dial must not
// regress the IPv4 first-attempt latency relative to the previous
// IPv4-then-IPv6 sequential implementation.
//
// Run with:
//
//	go test -bench BenchmarkDialLocal_IPv4 -benchmem ./cmd/client/...
//
// The benchmark holds a single 127.0.0.1 listener open for its
// entire duration; each iteration dials it and immediately closes
// the returned connection. The listener does not Accept — the
// kernel's accept queue absorbs the connections, which is fine for
// a dial-side latency benchmark.
func BenchmarkDialLocal_IPv4(b *testing.B) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen on 127.0.0.1: %v", err)
	}
	defer func() { _ = ln.Close() }()

	port := ln.Addr().(*net.TCPAddr).Port

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := dialLocal(port, 1*time.Second)
		if err != nil {
			b.Fatalf("dialLocal: %v", err)
		}
		_ = conn.Close()
	}
}
