package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestUpstreamHTTPS_SkipVerify validates Property P13: with
// --upstream-tls-skip-verify, the client successfully completes a TLS
// handshake against an upstream serving a self-signed certificate and a
// subsequent raw HTTP/1.1 request returns the upstream's response.
//
// Validates: Requirements 2.15
func TestUpstreamHTTPS_SkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "tls ok")
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	rawConn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial fake TLS server: %v", err)
	}
	t.Cleanup(func() { _ = rawConn.Close() })

	wrapped, err := wrapUpstreamTLS(rawConn, true, 5*time.Second)
	if err != nil {
		t.Fatalf("wrapUpstreamTLS with skipVerify=true: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = wrapped.Close() })

	// Send a minimal HTTP/1.1 request over the wrapped TLS conn.
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", u.Host)
	if err := wrapped.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := wrapped.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(wrapped), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "tls ok") {
		t.Fatalf("body: got %q, want contains %q", string(body), "tls ok")
	}
}

// TestUpstreamHTTPS_Verify validates Property P13: against an upstream with a
// self-signed certificate, the handshake fails when skipVerify is false and
// the error names the cause (certificate / unknown authority).
//
// Validates: Requirements 2.15
func TestUpstreamHTTPS_Verify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	rawConn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial fake TLS server: %v", err)
	}
	t.Cleanup(func() { _ = rawConn.Close() })

	_, err = wrapUpstreamTLS(rawConn, false, 5*time.Second)
	if err == nil {
		t.Fatalf("wrapUpstreamTLS with skipVerify=false: expected handshake error, got nil")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("error message: got %q, want contains %q", err.Error(), "certificate")
	}
}

// TestUpstreamHTTPS_Timeout validates Property P13: the handshake is bounded
// by the supplied timeout. We use net.Pipe to get an in-memory conn whose
// peer never responds, so the TLS handshake will block until the context
// deadline fires.
//
// Validates: Requirements 2.15
func TestUpstreamHTTPS_Timeout(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = clientSide.Close()
		_ = serverSide.Close()
	})

	start := time.Now()
	_, err := wrapUpstreamTLS(clientSide, true, 100*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	// Allow some slack but assert we returned roughly within the timeout.
	if elapsed > 2*time.Second {
		t.Fatalf("handshake did not respect timeout: elapsed=%v", elapsed)
	}
	msg := err.Error()
	if !strings.Contains(msg, "deadline exceeded") &&
		!strings.Contains(msg, "context deadline") &&
		!strings.Contains(msg, "timeout") {
		t.Fatalf("error message: got %q, want timeout/deadline indicator", msg)
	}
}

// portFromHostport extracts the integer port from a "host:port" string.
func portFromHostport(t *testing.T, hostport string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatalf("split host port %q: %v", hostport, err)
	}
	n, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return n
}

// TestProbeUpstreamScheme_HTTPS validates Property P13 + auto-detect: against
// an upstream serving TLS with a self-signed cert, the probe identifies the
// scheme as "https" and caches skipVerify=true (because dev certs are
// typically self-signed).
//
// Validates: Requirements 2.15
func TestProbeUpstreamScheme_HTTPS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port := portFromHostport(t, u.Host)

	scheme, skipVerify := probeUpstreamScheme(port)
	if scheme != "https" {
		t.Fatalf("scheme: got %q, want %q", scheme, "https")
	}
	if !skipVerify {
		t.Fatalf("skipVerify: got false, want true (dev certs are self-signed)")
	}
}

// TestProbeUpstreamScheme_HTTP validates the auto-detect path against a plain
// HTTP upstream — the TLS handshake fails fast (the server speaks HTTP/1.1,
// not TLS) and the probe returns ("http", false).
//
// Validates: Requirements 2.15
func TestProbeUpstreamScheme_HTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	port := portFromHostport(t, u.Host)

	start := time.Now()
	scheme, skipVerify := probeUpstreamScheme(port)
	elapsed := time.Since(start)

	if scheme != "http" {
		t.Fatalf("scheme: got %q, want %q", scheme, "http")
	}
	if skipVerify {
		t.Fatalf("skipVerify: got true, want false (no TLS to skip)")
	}
	// Probe should fail fast — bounded by the 200 ms handshake timeout.
	if elapsed > 2*time.Second {
		t.Fatalf("probe took too long: elapsed=%v", elapsed)
	}
}

// TestProbeUpstreamScheme_NoListener validates that the probe does not hang
// when nothing is listening on the port. The dial fails fast and the probe
// returns ("http", false) — the documented fallback. The actual stream dial
// will surface the real "no listener" error to the user later.
//
// Validates: Requirements 2.15
func TestProbeUpstreamScheme_NoListener(t *testing.T) {
	// Bind a listener to grab a free port, then close it so the port is
	// guaranteed to refuse connections (or at least have no listener) for
	// the duration of the test.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	start := time.Now()
	scheme, skipVerify := probeUpstreamScheme(port)
	elapsed := time.Since(start)

	if scheme != "http" {
		t.Fatalf("scheme: got %q, want %q (documented fallback when probe dial fails)", scheme, "http")
	}
	if skipVerify {
		t.Fatalf("skipVerify: got true, want false")
	}
	// Probe MUST not hang: total budget for the dial-fail path is ~1 s.
	if elapsed > 2*time.Second {
		t.Fatalf("probe hung on closed port: elapsed=%v", elapsed)
	}
}
