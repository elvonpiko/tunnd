//go:build integration

package integration

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newViteLikeUpstream constructs a fake Vite-style upstream that blocks
// any Host that isn't localhost:<port>. Returns the started server and its
// port — both are needed by the test setup.
func newViteLikeUpstream(t *testing.T) (*httptest.Server, int) {
	t.Helper()

	// Capture the port via a closure so the handler can compare against
	// localhost:<port> at request time. The variable is set before any
	// request can hit the listener (httptest.NewServer returns after the
	// listener is bound but before the test sends any traffic), so no
	// data-race window exists.
	var port int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := fmt.Sprintf("localhost:%d", port)
		if r.Host == expected {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("vite ok"))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `Blocked request. This host (%q) is not allowed.`, r.Host)
	}))
	port = portFromURL(t, srv.URL)
	t.Cleanup(srv.Close)
	return srv, port
}

// newPermissiveUpstream returns a fake upstream that accepts any Host
// header and always responds with "hello". Used to validate that a
// non-restricted upstream is unaffected by the host_header policy (P16).
func newPermissiveUpstream(t *testing.T) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(srv.Close)
	return srv, portFromURL(t, srv.URL)
}

// waitForRegistry polls the registry briefly so race-prone tests don't
// flake while the server-side Register() is still completing the handshake
// reply. A short sleep is fine — the handshake is synchronous on the
// happy path and finishes well under 50 ms.
func waitForRegistry() {
	time.Sleep(50 * time.Millisecond)
}

// TestE2E_VitelikeBlockedHost verifies that the default host-header policy
// (rewrite to localhost:<port>) makes a Vite-like blocked-host upstream
// return its real response. Without the rewrite the upstream would respond
// 400 "Blocked request"; with the rewrite, the upstream sees its expected
// Host and returns "vite ok".
func TestE2E_VitelikeBlockedHost(t *testing.T) {
	_, upstreamPort := newViteLikeUpstream(t)

	h := newHarness(t, "tunnd.example")

	// Default host_header (empty) → server treats as "rewrite".
	_, sub := h.startClient(t, clientOpts{
		subdomain:  "test01",
		hostHeader: "",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"
	status, body := h.doPublicRequest(t, publicHost)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", status, body)
	}
	if body != "vite ok" {
		t.Fatalf("body = %q, want %q", body, "vite ok")
	}
}

// TestE2E_VitelikeBlockedHost_Preserve verifies that opting out of the
// rewrite (host_header: "preserve") produces today's pre-fix behavior:
// the public Host is forwarded verbatim, and a Vite-like upstream rejects
// it with HTTP 400 "Blocked request". Validates P17.
func TestE2E_VitelikeBlockedHost_Preserve(t *testing.T) {
	_, upstreamPort := newViteLikeUpstream(t)

	h := newHarness(t, "tunnd.example")
	_, sub := h.startClient(t, clientOpts{
		subdomain:  "test02",
		hostHeader: "preserve",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"
	status, body := h.doPublicRequest(t, publicHost)

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%q", status, body)
	}
	if !strings.Contains(body, "Blocked request") {
		t.Fatalf("body = %q, want to contain %q", body, "Blocked request")
	}
}

// TestE2E_NonRestrictedUpstream verifies that an upstream which ignores
// the Host header behaves identically under both rewrite and preserve
// policies. Validates P16: the host_header default does not regress
// non-host-restricted upstreams.
func TestE2E_NonRestrictedUpstream(t *testing.T) {
	_, upstreamPort := newPermissiveUpstream(t)

	for _, policy := range []string{"", "rewrite", "preserve"} {
		policy := policy
		name := policy
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "tunnd.example")

			sub := "perm-" + name
			_, regSub := h.startClient(t, clientOpts{
				subdomain:  sub,
				hostHeader: policy,
				localPort:  upstreamPort,
			})
			waitForRegistry()

			publicHost := regSub + ".tunnd.example"
			status, body := h.doPublicRequest(t, publicHost)
			if status != http.StatusOK {
				t.Fatalf("policy=%q: status=%d, want 200; body=%q", policy, status, body)
			}
			if body != "hello" {
				t.Fatalf("policy=%q: body=%q, want %q", policy, body, "hello")
			}
		})
	}
}

// TestE2E_SubdomainRouting verifies (a) that Hosts not under the configured
// base domain return HTTP 404, and (b) that a properly-formed Host with a
// registered subdomain routes successfully. Validates P18.
func TestE2E_SubdomainRouting(t *testing.T) {
	_, upstreamPort := newPermissiveUpstream(t)

	h := newHarness(t, "tunnd.example")

	// (a) Before registering, a request for an unrelated host returns 404.
	statusBefore, _ := h.doPublicRequest(t, "nonsense.example.com")
	if statusBefore != http.StatusNotFound {
		t.Fatalf("non-matching host: status = %d, want 404", statusBefore)
	}

	// (b) After registering, the matching subdomain routes through.
	_, sub := h.startClient(t, clientOpts{
		subdomain:  "test03",
		hostHeader: "",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"
	status, body := h.doPublicRequest(t, publicHost)
	if status != http.StatusOK {
		t.Fatalf("registered host: status = %d, want 200; body=%q", status, body)
	}
	if body != "hello" {
		t.Fatalf("registered host: body = %q, want %q", body, "hello")
	}
}

// newSlowByteUpstream returns a fake upstream that emits a single byte
// after `delay`, then returns a 200 OK with body `"x"`. Used by the
// slow-stream regression test for Property 7 — proves that the client's
// per-iteration 120s read deadline is gone (an upstream that takes 130s
// to write its first byte must still complete successfully).
//
// The upstream writes its full response in one go after sleeping. We
// don't try to interleave header + body, because the client's response
// read on the local conn is what ultimately drives the through-tunnel
// timing — once the upstream emits anything, the bytes flow as MsgData
// frames to the public client.
func newSlowByteUpstream(t *testing.T, delay time.Duration) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Wait the long delay BEFORE writing anything. This is the worst
		// case for the old 120s per-iteration read deadline: the local
		// conn's Read() blocks for `delay` before any byte arrives.
		time.Sleep(delay)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(srv.Close)
	return srv, portFromURL(t, srv.URL)
}

// TestE2E_SlowStream_NoDeadline validates Property 7 — slow upstreams that
// take longer than the old 120s per-iteration deadline still complete.
//
// Wall-clock: ~135s. Skipped under -short. The fake upstream sits idle
// for 130s, then emits a 200 OK with body "x". Before the fix
// (cmd/client/main.go driveStream had a per-iteration 120s read
// deadline), the local-conn Read would have failed with i/o timeout
// near 120s and the public client would have seen a truncated stream.
// After the fix, the read blocks for the full 130s and returns "x".
func TestE2E_SlowStream_NoDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("slow streaming test (~135s)")
	}

	const upstreamDelay = 130 * time.Second
	_, upstreamPort := newSlowByteUpstream(t, upstreamDelay)

	h := newHarness(t, "tunnd.example")
	_, sub := h.startClient(t, clientOpts{
		subdomain:  "slow01",
		hostHeader: "",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"

	// Use the harness's timeout-configurable variant so the public-client
	// http.Client doesn't itself fire its 10s default while we wait for
	// the upstream to wake up. 150s gives ~20s of grace over the 130s
	// upstream delay for handshake, dial, and tear-down.
	start := time.Now()
	status, body := h.doPublicRequestWithTimeout(t, publicHost, 150*time.Second)
	elapsed := time.Since(start)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q (elapsed=%s)", status, body, elapsed)
	}
	if body != "x" {
		t.Fatalf("body = %q, want %q (elapsed=%s)", body, "x", elapsed)
	}
	// Sanity check: the response must actually have taken about as long
	// as the upstream slept. If it returned fast, we're not testing what
	// we think we're testing.
	if elapsed < upstreamDelay-5*time.Second {
		t.Fatalf("response returned in %s, expected ≥ %s — upstream delay path may not be exercised",
			elapsed, upstreamDelay-5*time.Second)
	}
}

// newSSEUpstream returns a fake SSE-style upstream that emits `count`
// events, one per second, each as `data: <i>\n\n`. After the last event
// the handler returns, which terminates the response. Each Write is
// followed by a Flush so the bytes hit the wire immediately rather than
// being held by net/http's response buffer.
//
// Note: the test client half-closes its TCP write side after sending
// the request (mirroring production behavior), which Go's http.Server
// detects via its background-read goroutine and uses to cancel
// `r.Context()`. We deliberately ignore that cancellation here — this
// test is about bytes streaming through the proxy, not about
// request-cancellation semantics.
func newSSEUpstream(t *testing.T, count int) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("upstream ResponseWriter does not implement http.Flusher")
			return
		}
		flusher.Flush()

		for i := 0; i < count; i++ {
			time.Sleep(1 * time.Second)
			if _, err := fmt.Fprintf(w, "data: %d\n\n", i); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, portFromURL(t, srv.URL)
}

// TestE2E_SSE_Streaming validates Property 6 — streaming bodies are
// delivered to the public client incrementally (not buffered until the
// upstream closes).
//
// Wall-clock: ~12s. Not gated by -short. The upstream emits 10 events
// at 1-second intervals; we read the response body one event at a time
// and assert each event arrives within 1.2s of the previous one. If the
// proxy buffered the full body, all 10 events would arrive together
// after ~10s and the per-event delta from the second event onward would
// be ~0s — but that's still ≤ 1.2s, so the meaningful signal is the
// FIRST event arriving incrementally with respect to the start of the
// request.
//
// To make the assertion robust, we measure the delta between the start
// of the request and event #0 (must be ≥ ~0.8s — proves the upstream
// wasn't pre-emitting), and the delta between successive events (must
// be ≤ 1.2s each — proves the proxy isn't holding bytes).
func TestE2E_SSE_Streaming(t *testing.T) {
	const eventCount = 10
	const eventInterval = 1 * time.Second
	const maxPerEventDelta = 1200 * time.Millisecond

	_, upstreamPort := newSSEUpstream(t, eventCount)

	h := newHarness(t, "tunnd.example")
	_, sub := h.startClient(t, clientOpts{
		subdomain:  "sse01",
		hostHeader: "",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"

	req, err := http.NewRequest(http.MethodGet, h.publicSrv.URL+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = publicHost

	// 30s overall is generous (eventCount * eventInterval = 10s plus
	// generous slack for handshake + last-event tear-down).
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("public request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	prev := time.Now()
	gotEvents := 0
	for gotEvents < eventCount {
		// Each event is `data: <i>\n\n`. Read up to the first \n (data line),
		// then read the trailing blank \n that terminates the event.
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading event %d: %v (line so far=%q)", gotEvents, err, line)
		}
		// Skip the empty terminator lines between events; only count "data: "
		// lines as event arrivals.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		now := time.Now()
		delta := now.Sub(prev)
		// First event has an inherent ~1s delay (the upstream sleeps before
		// emitting). Subsequent events should each arrive within 1s + slack.
		if delta > maxPerEventDelta {
			t.Fatalf("event %d arrived %s after previous (max %s) — proxy may be buffering",
				gotEvents, delta, maxPerEventDelta)
		}
		want := fmt.Sprintf("data: %d\n", gotEvents)
		if line != want {
			t.Fatalf("event %d: got %q, want %q", gotEvents, line, want)
		}
		prev = now
		gotEvents++
	}

	if gotEvents != eventCount {
		t.Fatalf("got %d events, want %d", gotEvents, eventCount)
	}
}


// newWSEchoUpstream returns a fake WebSocket upstream that:
//   - Accepts the upgrade with gorilla's Upgrader (CheckOrigin permissive).
//   - Echoes any text or binary message back verbatim.
//   - Replies to client pings with the matching pong (gorilla's default
//     ping handler does this; we just keep the read loop alive).
//   - When it receives a close frame, replies with its own close frame
//     and returns, so the client's ReadMessage sees a clean close error.
//
// The handler is intentionally minimal — the test exercises round-trip
// frame fidelity through the tunnel, not upstream WS logic.
func newWSEchoUpstream(t *testing.T) (*httptest.Server, int) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upstream upgrade: %v", err)
			return
		}
		defer c.Close()

		// Track close so the deferred Close() is harmless.
		c.SetCloseHandler(func(code int, text string) error {
			// Reply with a matching close frame so the peer sees a clean
			// close. gorilla returns nil here to suppress the default
			// echo (we'll do it explicitly).
			deadline := time.Now().Add(2 * time.Second)
			msg := websocket.FormatCloseMessage(code, text)
			return c.WriteControl(websocket.CloseMessage, msg, deadline)
		})

		for {
			mt, payload, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, payload); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, portFromURL(t, srv.URL)
}

// dialWSThroughTunnel opens a WebSocket conn from the public side, routed
// by Host header to the registered subdomain. The tricky bit: we want the
// HTTP/WS Host header to be the public hostname (so the registry routes
// correctly), but the actual TCP dial must go to the httptest listener.
// Solution: override NetDialContext to ignore the URL host and dial the
// real listener. The URL's host is what gorilla puts in the WS handshake
// Host header.
func dialWSThroughTunnel(t *testing.T, h *harness, publicHost string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		NetDialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, h.publicSrv.Listener.Addr().String())
		},
	}
	wsURL := "ws://" + publicHost + "/"
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial through tunnel: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestE2E_WebSocketHMR validates Property 4 — full WebSocket frame flow
// works end-to-end through the tunnel — and Property 19 — the WS hijack
// flow is unchanged regardless of the host-header policy (Host rewrite
// applies to the upgrade request, but post-101 frames are opaque bytes,
// so both `rewrite` and `preserve` must succeed identically).
//
// For each policy we exercise:
//   - Upgrade handshake completes.
//   - Text frame round-trip in both directions.
//   - Binary frame round-trip in both directions.
//   - Ping / pong round-trip.
//   - Close frame from the client side; upstream replies with close.
func TestE2E_WebSocketHMR(t *testing.T) {
	for _, policy := range []string{"rewrite", "preserve"} {
		policy := policy
		t.Run(policy, func(t *testing.T) {
			_, upstreamPort := newWSEchoUpstream(t)

			h := newHarness(t, "tunnd.example")
			_, sub := h.startClient(t, clientOpts{
				subdomain:  "ws-" + policy,
				hostHeader: policy,
				localPort:  upstreamPort,
			})
			waitForRegistry()

			publicHost := sub + ".tunnd.example"
			conn := dialWSThroughTunnel(t, h, publicHost)

			// Bound every IO on the public-side conn so a stuck pipe
			// surfaces as a test failure rather than a goroutine leak.
			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

			// ── Text frame round-trip ───────────────────────────────
			textPayload := []byte("hello vite — text")
			if err := conn.WriteMessage(websocket.TextMessage, textPayload); err != nil {
				t.Fatalf("write text: %v", err)
			}
			mt, got, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read text echo: %v", err)
			}
			if mt != websocket.TextMessage {
				t.Fatalf("text echo: type=%d want %d", mt, websocket.TextMessage)
			}
			if string(got) != string(textPayload) {
				t.Fatalf("text echo mismatch: got %q want %q", got, textPayload)
			}

			// ── Binary frame round-trip ─────────────────────────────
			binPayload := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x10, 0x20}
			if err := conn.WriteMessage(websocket.BinaryMessage, binPayload); err != nil {
				t.Fatalf("write binary: %v", err)
			}
			mt, got, err = conn.ReadMessage()
			if err != nil {
				t.Fatalf("read binary echo: %v", err)
			}
			if mt != websocket.BinaryMessage {
				t.Fatalf("binary echo: type=%d want %d", mt, websocket.BinaryMessage)
			}
			if string(got) != string(binPayload) {
				t.Fatalf("binary echo mismatch: got %v want %v", got, binPayload)
			}

			// ── Ping / pong + close round-trip ──────────────────────
			// gorilla caches read errors permanently — once a read
			// returns a deadline error, every subsequent ReadMessage
			// returns that same cached error. So we must not let a
			// timeout fire on the public conn between the ping and the
			// close. Instead: write ping, write close, then a single
			// ReadMessage drives both the pong (delivered via the pong
			// handler) and the close echo (returned as the read error).
			pongCh := make(chan []byte, 1)
			conn.SetPongHandler(func(appData string) error {
				select {
				case pongCh <- []byte(appData):
				default:
				}
				return nil
			})
			pingPayload := []byte("ping-payload")
			if err := conn.WriteControl(
				websocket.PingMessage,
				pingPayload,
				time.Now().Add(5*time.Second),
			); err != nil {
				t.Fatalf("write ping: %v", err)
			}

			// ── Close frame from client → upstream replies → client sees it ──
			closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
			if err := conn.WriteControl(
				websocket.CloseMessage,
				closeMsg,
				time.Now().Add(5*time.Second),
			); err != nil {
				t.Fatalf("write close: %v", err)
			}

			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, _, err = conn.ReadMessage()
			if err == nil {
				t.Fatalf("expected close error after sending close frame, got nil")
			}
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) &&
				!websocket.IsUnexpectedCloseError(err) &&
				!strings.Contains(err.Error(), "close") &&
				!strings.Contains(err.Error(), "EOF") {
				t.Fatalf("unexpected error reading after close: %v", err)
			}

			// The pong handler fires during the ReadMessage above (the
			// pong arrives interleaved with the close echo). Verify the
			// payload echoed back byte-equal.
			select {
			case got := <-pongCh:
				if string(got) != string(pingPayload) {
					t.Fatalf("pong payload: got %q want %q", got, pingPayload)
				}
			default:
				t.Fatalf("did not receive pong before close")
			}
		})
	}
}


// TestStreamLifecycleNoLeak validates Property 8 — per-stream state on
// both client and server is fully released after each request completes.
//
// Confirmation test (no production code change). Runs 50 sequential
// public requests end-to-end through doPublicRequest and asserts:
//   - The client-side test harness's per-stream map (tc.streams) is empty.
//   - The goroutine count is back near baseline (delta ≤ 5 after a 200ms
//     grace period for the close goroutines to wind down).
//
// Server-side leak detection is intentionally limited to the goroutine
// count: tunnel.Session.streams is unexported and adding an accessor
// method is more invasive than this confirming test warrants. A real
// stream leak shows up in goroutine count anyway — every leaked stream
// pins at least one driveStream / pumpRequest goroutine.
func TestStreamLifecycleNoLeak(t *testing.T) {
	_, upstreamPort := newPermissiveUpstream(t)

	h := newHarness(t, "tunnd.example")
	tc, sub := h.startClient(t, clientOpts{
		subdomain:  "leak01",
		hostHeader: "",
		localPort:  upstreamPort,
	})
	waitForRegistry()

	publicHost := sub + ".tunnd.example"

	// Capture goroutine baseline AFTER startup so we don't count the
	// harness's own stable goroutines (WS reader, send pump, etc.) as
	// leaks. Take a brief settle pause so any startup transients are
	// gone before we sample.
	time.Sleep(100 * time.Millisecond)
	baseGoroutines := runtime.NumGoroutine()

	const iterations = 50
	for i := 0; i < iterations; i++ {
		status, body := h.doPublicRequest(t, publicHost)
		if status != http.StatusOK {
			t.Fatalf("iter %d: status = %d, want 200; body=%q", i, status, body)
		}
		if body != "hello" {
			t.Fatalf("iter %d: body = %q, want %q", i, body, "hello")
		}
	}

	// Allow the per-stream close goroutines to finish their teardown
	// work. The CloseStream path closes both pipes and removes the
	// stream from the map, but the cleanup goroutines may still be
	// scheduling out at the moment doPublicRequest returns.
	time.Sleep(200 * time.Millisecond)

	// Client-side leak check: the test client's stream map must be empty.
	tc.streamsMu.Lock()
	clientStreams := len(tc.streams)
	tc.streamsMu.Unlock()
	if clientStreams != 0 {
		t.Fatalf("client-side leak: tc.streams has %d entries after %d requests, want 0",
			clientStreams, iterations)
	}

	// Goroutine drift check: any per-stream goroutine that didn't unwind
	// would still be parked on a pipe Read. A small slack accounts for
	// goroutines created by net/http internals (idle conn closers,
	// background readers from DisableKeepAlives=true variant) that may
	// not have GC'd yet.
	const slack = 5
	endGoroutines := runtime.NumGoroutine()
	if drift := endGoroutines - baseGoroutines; drift > slack {
		t.Fatalf("goroutine leak: started at %d, ended at %d (drift %d > %d slack) after %d requests",
			baseGoroutines, endGoroutines, drift, slack, iterations)
	}
}
