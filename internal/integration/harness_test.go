//go:build integration

// Package integration contains end-to-end integration tests for tunnd.
//
// These tests are gated by the `integration` build tag so the default
// `go test ./...` does NOT run them. Run the suite explicitly with:
//
//	go test -tags=integration ./internal/integration/...
//
// The harness spins up a real tunnd-server and a minimal in-process
// client (mirroring just the WebSocket dial + register payload + per-stream
// pump logic from cmd/client) so a public HTTP request flows the full
// path: public listener → registry → WS control plane → test client →
// fake upstream → back through the same wire.
package integration

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/control"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
	"github.com/elvonpiko/tunnd/pkg/proto"
)

// harness holds the shared in-process server pieces for one integration test.
type harness struct {
	db         *store.DB
	registry   *tunnel.Registry
	authSvc    *auth.Service
	publicSrv  *httptest.Server // mounts the registry — public traffic listener
	controlSrv *httptest.Server // mounts the control handler — WS dial target
	domain     string
	tokenValue string
}

// newHarness builds a fresh in-process tunnd-server (registry + control plane +
// public listener) backed by a temp SQLite DB and a freshly-issued auth token.
// All pieces are torn down via t.Cleanup.
func newHarness(t *testing.T, domain string) *harness {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "tunnd-itest-*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	f.Close()

	db, err := store.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	authSvc := auth.New(db)
	tok, err := authSvc.CreateToken("integration-test", 0)
	if err != nil {
		db.Close()
		t.Fatalf("create token: %v", err)
	}

	registry := tunnel.New(db, domain)

	controlSrv := httptest.NewServer(control.New(authSvc, registry, domain))
	publicSrv := httptest.NewServer(registry)

	h := &harness{
		db:         db,
		registry:   registry,
		authSvc:    authSvc,
		publicSrv:  publicSrv,
		controlSrv: controlSrv,
		domain:     domain,
		tokenValue: tok.Value,
	}
	t.Cleanup(h.Close)
	return h
}

// Close tears down both httptest servers and the underlying DB. Idempotent.
//
// Ordering matters: when the WS conn closes, the server-side handler fires
// `Registry.Deregister` (which writes to the DB) in a deferred path. We must
// drain that work BEFORE closing the DB, otherwise the "readonly database"
// errors and stray WAL/SHM files break the t.TempDir RemoveAll cleanup.
//
// `Deregister` deletes the session from the registry map BEFORE writing to
// the DB, so polling `ActiveSessions` alone is not sufficient — we add a
// short grace sleep on top to cover the brief window between map delete and
// DB write.
func (h *harness) Close() {
	if h.controlSrv != nil {
		h.controlSrv.Close()
		h.controlSrv = nil
	}
	if h.publicSrv != nil {
		h.publicSrv.Close()
		h.publicSrv = nil
	}
	if h.registry != nil {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if len(h.registry.ActiveSessions()) == 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		// Grace window for the deferred DB write inside Deregister to land.
		time.Sleep(100 * time.Millisecond)
		h.registry = nil
	}
	if h.db != nil {
		h.db.Close()
		h.db = nil
	}
}

// wsURL converts the control httptest URL ("http://addr") into the
// ws:// URL the client dials.
func (h *harness) wsURL() string {
	u := strings.Replace(h.controlSrv.URL, "http://", "ws://", 1)
	return u + "/_tunnd/control"
}

// clientOpts are the knobs a test passes when starting an in-process client.
type clientOpts struct {
	subdomain  string // requested subdomain (e.g. "test01")
	hostHeader string // "" / "rewrite" / "preserve" / literal hostname
	localPort  int    // upstream port to forward to (127.0.0.1:<localPort>)
}

// testStream is one in-flight tunneled request on the client side.
type testStream struct {
	id   string
	reqR *io.PipeReader
	reqW *io.PipeWriter
}

// testClient is an in-process tunnd client that mirrors just enough of
// cmd/client/main.go to drive the WS control plane: handshake, register,
// per-stream open/data/req_done/close pumping. Intentionally minimal —
// no inspector, no flags, no reconnect.
type testClient struct {
	conn      *websocket.Conn
	localPort int

	writeMu sync.Mutex // serialises WS writes (gorilla is not concurrent-safe)

	streamsMu sync.Mutex
	streams   map[string]*testStream

	closeOnce sync.Once
	closed    chan struct{}

	t *testing.T
}

// startClient opens a WS to the control plane, sends MsgRegister, waits for
// MsgRegistered, and returns the live client + the registered subdomain.
// The reader loop runs in a goroutine until t.Cleanup tears it down.
func (h *harness) startClient(t *testing.T, opts clientOpts) (*testClient, string) {
	t.Helper()

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(h.wsURL(), nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}

	// Register.
	regMsg, err := proto.EncodeJSON(proto.MsgRegister, proto.RegisterPayload{
		Token:      h.tokenValue,
		Subdomain:  opts.subdomain,
		Protocol:   "http",
		LocalPort:  opts.localPort,
		HostHeader: opts.hostHeader,
	})
	if err != nil {
		conn.Close()
		t.Fatalf("encode register: %v", err)
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.BinaryMessage, regMsg); err != nil {
		conn.Close()
		t.Fatalf("send register: %v", err)
	}

	// Receive MsgRegistered (or MsgError).
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		t.Fatalf("read register reply: %v", err)
	}
	conn.SetReadDeadline(time.Time{})

	if proto.FrameKind(raw) != proto.FrameKindJSON {
		conn.Close()
		t.Fatalf("expected JSON frame, got kind 0x%02x", proto.FrameKind(raw))
	}
	env, err := proto.DecodeJSON(raw)
	if err != nil {
		conn.Close()
		t.Fatalf("decode register reply: %v", err)
	}
	if env.Type == proto.MsgError {
		var ep proto.ErrorPayload
		_ = proto.DecodePayload(env, &ep)
		conn.Close()
		t.Fatalf("server rejected register: %s — %s", ep.Code, ep.Message)
	}
	if env.Type != proto.MsgRegistered {
		conn.Close()
		t.Fatalf("expected MsgRegistered, got %s", env.Type)
	}
	var reg proto.RegisteredPayload
	if err := proto.DecodePayload(env, &reg); err != nil {
		conn.Close()
		t.Fatalf("decode registered payload: %v", err)
	}

	tc := &testClient{
		conn:      conn,
		localPort: opts.localPort,
		streams:   make(map[string]*testStream),
		closed:    make(chan struct{}),
		t:         t,
	}

	// Long-running tests (e.g. slow-stream emitting one byte after 130s)
	// keep the WS conn idle from our perspective, so we extend the read
	// deadline on every server ping (default ping handler responds with
	// pong but doesn't update our deadline). pongWait of 90s comfortably
	// covers the server's 54s ping period plus jitter.
	const pongWait = 90 * time.Second
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		// Extend our read deadline so a >120s in-flight stream survives
		// a long upstream-side wait.
		conn.SetReadDeadline(time.Now().Add(pongWait))
		// Mirror gorilla's default ping handler: reply with a pong frame.
		err := conn.WriteControl(
			websocket.PongMessage,
			[]byte(appData),
			time.Now().Add(10*time.Second),
		)
		if err == websocket.ErrCloseSent {
			return nil
		}
		return err
	})

	go tc.readLoop()

	t.Cleanup(tc.Close)
	return tc, reg.Subdomain
}

// Close shuts down the WS conn and signals the reader to stop. Idempotent.
func (tc *testClient) Close() {
	tc.closeOnce.Do(func() {
		close(tc.closed)
		tc.conn.Close()
		tc.streamsMu.Lock()
		for _, s := range tc.streams {
			s.reqW.Close()
			s.reqR.Close()
		}
		tc.streams = map[string]*testStream{}
		tc.streamsMu.Unlock()
	})
}

// readLoop processes incoming control-plane frames until the conn closes.
// It mirrors the dispatch logic in cmd/client/main.go's readLoop, minimally.
func (tc *testClient) readLoop() {
	for {
		_, raw, err := tc.conn.ReadMessage()
		if err != nil {
			return
		}

		if proto.FrameKind(raw) == proto.FrameKindBinaryData {
			streamID, payload, derr := proto.DecodeBinaryData(raw)
			if derr != nil {
				continue
			}
			tc.streamsMu.Lock()
			s := tc.streams[streamID]
			tc.streamsMu.Unlock()
			if s != nil {
				// Best-effort write — pipe close races with cleanup are fine.
				_, _ = s.reqW.Write(payload)
			}
			continue
		}

		env, err := proto.DecodeJSON(raw)
		if err != nil {
			continue
		}
		switch env.Type {
		case proto.MsgOpen:
			var op proto.OpenPayload
			if err := proto.DecodePayload(env, &op); err != nil {
				continue
			}
			tc.openStream(op.StreamID)

		case proto.MsgOpenTCP:
			// Used by handleUpgrade for WS / HTTP-upgrade traffic. After
			// the upgrade handshake the wire is opaque bytes — the test
			// client must pump in both directions concurrently.
			var op proto.OpenPayload
			if err := proto.DecodePayload(env, &op); err != nil {
				continue
			}
			tc.openTCPStream(op.StreamID)

		case proto.MsgReqDone:
			var rd proto.ReqDonePayload
			if err := proto.DecodePayload(env, &rd); err != nil {
				continue
			}
			tc.streamsMu.Lock()
			s := tc.streams[rd.StreamID]
			tc.streamsMu.Unlock()
			if s != nil {
				// Closing the write side EOFs reqR — the pump goroutine
				// will then half-close the local conn's write side.
				_ = s.reqW.Close()
			}

		case proto.MsgClose:
			var cp proto.ClosePayload
			if err := proto.DecodePayload(env, &cp); err != nil {
				continue
			}
			tc.removeStream(cp.StreamID)

		case proto.MsgPong, proto.MsgError:
			// nothing — error frames mid-session are rare and tests don't
			// exercise them; pong is handled by SetPongHandler.
		}
	}
}

// openStream registers a new stream for streamID and spawns the per-stream
// driver goroutine. Synchronous registration ensures any binary data frames
// that arrive immediately after MsgOpen are routed correctly.
func (tc *testClient) openStream(streamID string) {
	reqR, reqW := io.Pipe()
	s := &testStream{id: streamID, reqR: reqR, reqW: reqW}
	tc.streamsMu.Lock()
	tc.streams[streamID] = s
	tc.streamsMu.Unlock()
	go tc.driveStream(s)
}

// driveStream dials the local upstream, pumps request bytes in, and forwards
// the response bytes back over the WS as binary data frames. Mirrors the
// minimal subset of cmd/client.driveStream needed for the four tests in
// this package.
func (tc *testClient) driveStream(s *testStream) {
	defer func() {
		tc.removeStream(s.id)
		// Tell the server this stream is done — server closes its response
		// pipe so ServeHTTP's body Copy can complete.
		closeMsg, _ := proto.EncodeJSON(proto.MsgClose, proto.ClosePayload{StreamID: s.id})
		_ = tc.writeFrame(closeMsg)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", tc.localPort)
	localConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		// Drain whatever the server queues so the pipe doesn't block.
		s.reqW.CloseWithError(err)
		// Read and discard any pending request bytes until reqR EOFs.
		_, _ = io.Copy(io.Discard, s.reqR)
		return
	}
	defer localConn.Close()

	// Pump request bytes from reqR → local conn. When reqR EOFs (server sent
	// MsgReqDone, which closes reqW in our reader loop), io.Copy returns and
	// we half-close the local conn's write side.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = io.Copy(localConn, s.reqR)
		if tcpConn, ok := localConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()
	<-pumpDone

	// Read response from local conn → send as binary data frames. EOF means
	// upstream closed the conn (Connection: close set server-side), which is
	// our signal that the response is fully delivered.
	buf := make([]byte, 32*1024)
	for {
		n, rerr := localConn.Read(buf)
		if n > 0 {
			frame, ferr := proto.EncodeBinaryData(s.id, buf[:n])
			if ferr != nil {
				return
			}
			if werr := tc.writeFrame(frame); werr != nil {
				return
			}
		}
		if rerr != nil {
			return
		}
	}
}

// writeFrame sends a frame over the WS, holding writeMu so concurrent
// per-stream goroutines don't interleave gorilla's non-thread-safe writes.
func (tc *testClient) writeFrame(frame []byte) error {
	tc.writeMu.Lock()
	defer tc.writeMu.Unlock()
	tc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return tc.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// openTCPStream registers a TCP-style stream (used by HTTP upgrade — WS) and
// spawns the driver. Mirrors openStream but uses the bidirectional driver.
func (tc *testClient) openTCPStream(streamID string) {
	reqR, reqW := io.Pipe()
	s := &testStream{id: streamID, reqR: reqR, reqW: reqW}
	tc.streamsMu.Lock()
	tc.streams[streamID] = s
	tc.streamsMu.Unlock()
	go tc.driveTCPStream(s)
}

// driveTCPStream dials the local upstream and pipes bytes in both directions
// concurrently. No MsgReqDone is ever sent — TCP streams have no request /
// response boundary. On either side closing, send MsgClose so the server
// releases its half of the stream.
func (tc *testClient) driveTCPStream(s *testStream) {
	defer func() {
		tc.removeStream(s.id)
		closeMsg, _ := proto.EncodeJSON(proto.MsgClose, proto.ClosePayload{StreamID: s.id})
		_ = tc.writeFrame(closeMsg)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", tc.localPort)
	localConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		s.reqW.CloseWithError(err)
		_, _ = io.Copy(io.Discard, s.reqR)
		return
	}
	defer localConn.Close()

	// reqR (server → us) → local conn (us → upstream)
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = io.Copy(localConn, s.reqR)
		// On EOF (server closed its end), half-close so upstream sees it.
		if tcpConn, ok := localConn.(*net.TCPConn); ok {
			_ = tcpConn.CloseWrite()
		}
	}()

	// local conn (upstream → us) → MsgData frames (us → server)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := localConn.Read(buf)
		if n > 0 {
			frame, ferr := proto.EncodeBinaryData(s.id, buf[:n])
			if ferr != nil {
				break
			}
			if werr := tc.writeFrame(frame); werr != nil {
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	<-pumpDone
}

// removeStream deletes a stream entry and closes its pipes.
func (tc *testClient) removeStream(id string) {
	tc.streamsMu.Lock()
	s, ok := tc.streams[id]
	if ok {
		delete(tc.streams, id)
	}
	tc.streamsMu.Unlock()
	if ok && s != nil {
		_ = s.reqR.Close()
		_ = s.reqW.Close()
	}
}

// portFromURL extracts the numeric port from an httptest.NewServer URL
// (e.g. "http://127.0.0.1:54321" → 54321). Tests use this to feed
// LocalPort into clientOpts.
func portFromURL(t *testing.T, raw string) int {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port from %q: %v", raw, err)
	}
	return p
}

// doPublicRequest sends an HTTP GET to the public listener with a custom
// Host header and returns the status code + body. Uses a fresh client
// (no idle connection reuse) so each call exercises a clean code path.
func (h *harness) doPublicRequest(t *testing.T, host string) (int, string) {
	return h.doPublicRequestWithTimeout(t, host, 10*time.Second)
}

// doPublicRequestWithTimeout is doPublicRequest with a configurable timeout.
// Long-streaming tests (e.g. the 130s slow-stream scenario) need a much
// larger timeout than the 10s default.
func (h *harness) doPublicRequestWithTimeout(t *testing.T, host string, timeout time.Duration) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.publicSrv.URL+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = host

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("public request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}
