// Package tunnel manages the in-memory registry of active tunnels and
// the bidirectional HTTP proxy that forwards public traffic to clients.
//
// Data flow (one HTTP request):
//
//	Browser → Server (ServeHTTP)
//	  └─ opens Stream{reqW→reqR, respW→respR}
//	  └─ sends MsgOpen(streamID) to client via WebSocket
//	  └─ goroutine: writes raw HTTP request bytes → reqW (→ reqR)
//	  └─ blocks reading HTTP response from respR
//
//	Client (openStream goroutine)
//	  └─ receives MsgOpen → dials localhost:port
//	  └─ receives MsgData frames → writes to local conn (request bytes)
//	  └─ reads local conn response bytes → sends MsgData frames back
//
//	Server (control reader goroutine)
//	  └─ receives MsgData → writes to respW (→ respR)
//	  └─ ServeHTTP unblocks: reads HTTP response from respR → writes to browser
package tunnel

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/pkg/proto"
)

// Stream holds the two independent pipes for one request/response cycle.
//
//   - req:  server writes the serialised HTTP request; client reads via MsgData frames.
//   - resp: client writes the raw HTTP response via MsgData frames; server reads to reply.
type Stream struct {
	id string

	// Request pipe: server → client
	reqR *io.PipeReader // client data-pump reads from here and sends MsgData
	reqW *io.PipeWriter // ServeHTTP writes the serialised request here

	// Response pipe: client → server
	respR *io.PipeReader // ServeHTTP reads the HTTP response from here
	respW *io.PipeWriter // control-plane reader writes MsgData bytes here
}

// Session represents one connected client and its registered tunnel.
type Session struct {
	ID        string
	TunnelID  string
	TokenID   string
	Subdomain string
	Protocol  string
	PublicURL string

	// TCPListener is the public listener for TCP tunnels (nil for HTTP).
	// When the session is deregistered, this listener is closed which causes
	// the accept loop to exit and free the port.
	TCPListener net.Listener
	TCPPort     int

	// send queues outgoing frames for the WebSocket writer goroutine.
	send chan []byte

	mu      sync.Mutex
	streams map[string]*Stream
}

// defaultReservedSubdomains is the set of subdomains that cannot be registered
// unless the server operator explicitly overrides the reserved list.
var defaultReservedSubdomains = []string{"www", "api", "admin", "mail", "ftp"}

// Registry holds all active tunnel sessions keyed by subdomain.
type Registry struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	db        *store.DB
	domain    string
	validator *SubdomainValidator

	// TCP port allocation
	tcpMu        sync.Mutex
	tcpPortInUse map[int]bool
	tcpMinPort   int
	tcpMaxPort   int
}

// New returns an initialised Registry with the default reserved subdomain list.
func New(db *store.DB, domain string) *Registry {
	return NewWithValidator(db, domain, defaultReservedSubdomains)
}

// NewWithValidator returns a Registry that uses the provided reserved subdomain list
// to initialise its SubdomainValidator. Pass nil or an empty slice to disable
// reserved-name checking.
func NewWithValidator(db *store.DB, domain string, reservedSubdomains []string) *Registry {
	if reservedSubdomains == nil {
		reservedSubdomains = defaultReservedSubdomains
	}
	return &Registry{
		sessions:     make(map[string]*Session),
		db:           db,
		domain:       domain,
		validator:    NewSubdomainValidator(reservedSubdomains),
		tcpPortInUse: make(map[int]bool),
		tcpMinPort:   20000,
		tcpMaxPort:   20100,
	}
}

// SetTCPPortRange configures the inclusive [min, max] port range from which
// TCP tunnels allocate public ports. Must be called before any tunnel
// registers. If the range is invalid, the existing range is kept.
func (r *Registry) SetTCPPortRange(minPort, maxPort int) {
	if minPort < 1 || maxPort < 1 || minPort > maxPort {
		return
	}
	r.tcpMu.Lock()
	r.tcpMinPort = minPort
	r.tcpMaxPort = maxPort
	r.tcpMu.Unlock()
}

// ── Registration ──────────────────────────────────────────────────────────────

// Register creates a new Session. If subdomain is empty a random one is chosen.
// For TCP tunnels, allocates a public port from the configured range and binds
// a listener; the listener is closed automatically when the session is
// deregistered.
func (r *Registry) Register(tokenID, subdomain, protocol string, localPort int) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if subdomain == "" {
		// Generate a random subdomain and ensure it is not already taken.
		for {
			candidate := randomSubdomain()
			if _, exists := r.sessions[candidate]; !exists {
				subdomain = candidate
				break
			}
		}
	} else {
		// Validate and sanitize the requested custom subdomain.
		sanitized, err := r.validator.ValidateAndSanitize(subdomain)
		if err != nil {
			return nil, err
		}
		subdomain = sanitized
	}

	// Check for conflicts with existing sessions.
	if _, exists := r.sessions[subdomain]; exists {
		return nil, &ValidationError{
			Code:    "subdomain_in_use",
			Message: fmt.Sprintf("subdomain '%s' is already in use", subdomain),
		}
	}

	tunnelID := uuid.New().String()

	// ── TCP-specific setup ────────────────────────────────────────────────────
	var (
		tcpListener net.Listener
		tcpPort     int
		publicURL   string
	)
	if protocol == "tcp" {
		port, ln, err := r.allocateTCPPort()
		if err != nil {
			return nil, &ValidationError{Code: "tcp_port_unavailable", Message: err.Error()}
		}
		tcpListener = ln
		tcpPort = port
		publicURL = fmt.Sprintf("tcp://%s:%d", r.domain, tcpPort)
	} else {
		publicURL = fmt.Sprintf("https://%s.%s", subdomain, r.domain)
	}

	sess := &Session{
		ID:          uuid.New().String(),
		TunnelID:    tunnelID,
		TokenID:     tokenID,
		Subdomain:   subdomain,
		Protocol:    protocol,
		PublicURL:   publicURL,
		TCPListener: tcpListener,
		TCPPort:     tcpPort,
		send:        make(chan []byte, 512),
		streams:     make(map[string]*Stream),
	}

	if err := r.db.OpenTunnel(&store.TunnelRecord{
		ID:        tunnelID,
		TokenID:   tokenID,
		Subdomain: subdomain,
		Protocol:  protocol,
		PublicURL: publicURL,
		LocalPort: localPort,
		OpenedAt:  time.Now(),
	}); err != nil {
		if tcpListener != nil {
			tcpListener.Close() //nolint:errcheck
			r.releaseTCPPort(tcpPort)
		}
		return nil, fmt.Errorf("persisting tunnel: %w", err)
	}

	r.sessions[subdomain] = sess

	// Spawn the TCP accept loop now that the session is fully wired up.
	if tcpListener != nil {
		go r.acceptTCPLoop(sess)
	}

	log.Info().
		Str("subdomain", subdomain).
		Str("public_url", publicURL).
		Str("protocol", protocol).
		Int("tcp_port", tcpPort).
		Msg("tunnel registered")

	return sess, nil
}

// allocateTCPPort scans the configured TCP port range for a free slot,
// binds a listener, and returns it. The caller is responsible for closing
// the listener and calling releaseTCPPort when done.
func (r *Registry) allocateTCPPort() (int, net.Listener, error) {
	r.tcpMu.Lock()
	minP, maxP := r.tcpMinPort, r.tcpMaxPort
	// Snapshot reserved set under lock to avoid scanning racing.
	reserved := make(map[int]bool, len(r.tcpPortInUse))
	for k, v := range r.tcpPortInUse {
		reserved[k] = v
	}
	r.tcpMu.Unlock()

	for port := minP; port <= maxP; port++ {
		if reserved[port] {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue // probably bound by another process; skip
		}
		// Mark as reserved.
		r.tcpMu.Lock()
		if r.tcpPortInUse[port] {
			r.tcpMu.Unlock()
			ln.Close() //nolint:errcheck
			continue
		}
		r.tcpPortInUse[port] = true
		r.tcpMu.Unlock()
		return port, ln, nil
	}
	return 0, nil, fmt.Errorf(
		"no free TCP port in range %d–%d (server is at capacity)", minP, maxP,
	)
}

func (r *Registry) releaseTCPPort(port int) {
	if port == 0 {
		return
	}
	r.tcpMu.Lock()
	delete(r.tcpPortInUse, port)
	r.tcpMu.Unlock()
}

// acceptTCPLoop accepts inbound TCP connections on a session's public
// listener and proxies each one through the WebSocket as a new stream.
func (r *Registry) acceptTCPLoop(sess *Session) {
	defer log.Info().Str("subdomain", sess.Subdomain).Int("port", sess.TCPPort).
		Msg("TCP accept loop stopped")

	for {
		inbound, err := sess.TCPListener.Accept()
		if err != nil {
			// Listener closed (deregister) — clean exit.
			return
		}
		go r.proxyTCPConn(sess, inbound)
	}
}

// proxyTCPConn opens a TCP stream on the session, notifies the client to
// dial the local TCP service, and pipes bytes in both directions.
func (r *Registry) proxyTCPConn(sess *Session, inbound net.Conn) {
	defer inbound.Close() //nolint:errcheck

	st, err := sess.openTCPStream()
	if err != nil {
		log.Error().Err(err).Str("session", sess.ID).Msg("openTCPStream failed")
		return
	}
	defer sess.CloseStream(st.id)

	// inbound → client
	go func() {
		defer st.reqW.Close() //nolint:errcheck
		buf := make([]byte, 32*1024)
		for {
			n, err := inbound.Read(buf)
			if n > 0 {
				if _, werr := st.reqW.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// client → inbound
	buf := make([]byte, 32*1024)
	for {
		n, err := st.respR.Read(buf)
		if n > 0 {
			if _, werr := inbound.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Deregister removes a session from the registry and marks the tunnel closed.
func (r *Registry) Deregister(subdomain string) {
	r.mu.Lock()
	sess, exists := r.sessions[subdomain]
	if exists {
		delete(r.sessions, subdomain)
	}
	r.mu.Unlock()

	if !exists {
		return
	}

	// Close TCP listener (if any) so the accept loop exits and the port frees.
	if sess.TCPListener != nil {
		sess.TCPListener.Close() //nolint:errcheck
		r.releaseTCPPort(sess.TCPPort)
	}

	// Tear down any in-flight streams so ServeHTTP goroutines unblock.
	sess.mu.Lock()
	for _, st := range sess.streams {
		st.reqW.CloseWithError(io.ErrClosedPipe)
		st.respW.CloseWithError(io.ErrClosedPipe)
	}
	sess.mu.Unlock()

	if err := r.db.CloseTunnel(sess.TunnelID); err != nil {
		log.Error().Err(err).Str("tunnel_id", sess.TunnelID).Msg("closing tunnel in DB")
	}
	log.Info().Str("subdomain", subdomain).Msg("tunnel deregistered")
}

// Lookup returns the Session for a subdomain, or nil if not found.
func (r *Registry) Lookup(subdomain string) *Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[subdomain]
}

// ActiveSessions returns a snapshot of all live sessions.
func (r *Registry) ActiveSessions() []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

// ── Session messaging ─────────────────────────────────────────────────────────

// Send enqueues a control-plane frame for the WebSocket writer goroutine.
// It blocks if the send buffer is full, since dropping a frame would silently
// corrupt the stream protocol (a single dropped MsgData / MsgReqDone /
// MsgClose hangs the request indefinitely on the other side). A 30s deadline
// guards against a permanently stuck writer (e.g., dead client).
func (s *Session) Send(msg []byte) {
	select {
	case s.send <- msg:
		return
	default:
	}
	// Buffer is full — block briefly. If the writer is healthy, slots
	// free up quickly. If not, give up so we don't pin the caller forever.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case s.send <- msg:
	case <-timer.C:
		log.Warn().Str("session", s.ID).Msg("send blocked >30s — dropping frame (slow client?)")
	}
}

// SendCh exposes the send channel to the WebSocket writer goroutine.
func (s *Session) SendCh() <-chan []byte { return s.send }

// ── Stream management ─────────────────────────────────────────────────────────

// newStream allocates a Stream with two independent pipes.
func newStream() *Stream {
	reqR, reqW := io.Pipe()
	respR, respW := io.Pipe()
	return &Stream{
		id:    uuid.New().String(),
		reqR:  reqR,
		reqW:  reqW,
		respR: respR,
		respW: respW,
	}
}

// openStream creates a stream, registers it, notifies the client, and starts
// the goroutine that pumps request bytes from reqR → MsgData → client.
// ServeHTTP calls this then immediately blocks reading from st.respR.
func (s *Session) openStream() (*Stream, error) {
	st := newStream()

	s.mu.Lock()
	s.streams[st.id] = st
	s.mu.Unlock()

	// Notify client: a new request is arriving, open a local connection for it.
	msg, err := proto.EncodeJSON(proto.MsgOpen, proto.OpenPayload{StreamID: st.id})
	if err != nil {
		s.removeStream(st.id)
		st.reqW.CloseWithError(err)
		st.respW.CloseWithError(err)
		return nil, err
	}
	s.Send(msg)

	// Pump request bytes from reqR → MsgData frames → client.
	// reqW is written by ServeHTTP after this returns.
	go s.pumpRequest(st)

	return st, nil
}

// openTCPStream creates a stream for a TCP tunnel and notifies the client to
// dial its local TCP service. Unlike openStream, no MsgReqDone is ever sent —
// TCP streams are fully bidirectional with the same DataPump used for both
// directions.
func (s *Session) openTCPStream() (*Stream, error) {
	st := newStream()

	s.mu.Lock()
	s.streams[st.id] = st
	s.mu.Unlock()

	msg, err := proto.EncodeJSON(proto.MsgOpenTCP, proto.OpenPayload{StreamID: st.id})
	if err != nil {
		s.removeStream(st.id)
		st.reqW.CloseWithError(err)
		st.respW.CloseWithError(err)
		return nil, err
	}
	s.Send(msg)

	// Pump bytes from reqR (inbound TCP conn → server) → client as MsgData.
	go s.pumpTCP(st)

	return st, nil
}

// pumpTCP forwards bytes from st.reqR to the client as binary data frames,
// without sending MsgReqDone at EOF (TCP streams have no request/response
// distinction). On EOF, sends a MsgClose so the client can release its
// local conn for this stream.
func (s *Session) pumpTCP(st *Stream) {
	buf := make([]byte, 32*1024)
	for {
		n, err := st.reqR.Read(buf)
		if n > 0 {
			frame, encErr := proto.EncodeBinaryData(st.id, buf[:n])
			if encErr != nil {
				log.Error().Err(encErr).Msg("encoding binary data (tcp)")
				return
			}
			s.Send(frame)
		}
		if err != nil {
			break
		}
	}
	// Tell client: no more bytes will be sent on this stream.
	msg, _ := proto.EncodeJSON(proto.MsgClose, proto.ClosePayload{StreamID: st.id})
	s.Send(msg)
}

// pumpRequest reads serialised HTTP request bytes from st.reqR and forwards
// them to the client as binary data frames. When reqR is closed (EOF) it
// sends MsgReqDone (a JSON envelope) so the client knows the request is fully
// received and it can start reading the local response.
func (s *Session) pumpRequest(st *Stream) {
	buf := make([]byte, 32*1024)
	for {
		n, err := st.reqR.Read(buf)
		if n > 0 {
			frame, encErr := proto.EncodeBinaryData(st.id, buf[:n])
			if encErr != nil {
				log.Error().Err(encErr).Msg("encoding binary data")
				break
			}
			s.Send(frame)
		}
		if err != nil {
			break // EOF or pipe closed
		}
	}
	// Signal to client: full request has been sent, start reading local response.
	msg, _ := proto.EncodeJSON(proto.MsgReqDone, proto.ReqDonePayload{StreamID: st.id})
	s.Send(msg)
}

// WriteRespData delivers bytes from a client MsgData frame into a stream's
// response pipe. Called by the control-plane reader goroutine.
func (s *Session) WriteRespData(streamID string, data []byte) {
	s.mu.Lock()
	st, ok := s.streams[streamID]
	s.mu.Unlock()
	if !ok {
		return
	}
	if _, err := st.respW.Write(data); err != nil {
		log.Debug().Err(err).Str("stream", streamID).Msg("resp pipe write error")
	}
}

// CloseStream closes both pipes for a stream and removes it.
// Called when the client sends MsgClose (response fully sent).
func (s *Session) CloseStream(streamID string) {
	s.mu.Lock()
	st, ok := s.streams[streamID]
	if ok {
		delete(s.streams, streamID)
	}
	s.mu.Unlock()
	if ok {
		st.respW.Close() // unblocks ServeHTTP's http.ReadResponse
		st.reqW.Close()
	}
}

func (s *Session) removeStream(id string) {
	s.mu.Lock()
	delete(s.streams, id)
	s.mu.Unlock()
}

// ── HTTP reverse-proxy ────────────────────────────────────────────────────────

// ServeHTTP satisfies http.Handler. Called for every inbound public request.
//
// It:
//  1. Looks up the tunnel session for the subdomain in the Host header.
//  2. Opens a stream (allocates pipes, notifies client via MsgOpen).
//  3. Writes the serialised HTTP request into the stream's request pipe.
//  4. Blocks reading the HTTP response from the stream's response pipe
//     (fed by MsgData frames arriving from the client).
//  5. Copies the response back to the browser.
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	subdomain := extractSubdomain(req.Host, r.domain)
	log.Debug().Str("host", req.Host).Str("subdomain", subdomain).Msg("ServeHTTP called")
	if subdomain == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	sess := r.Lookup(subdomain)
	if sess == nil {
		http.Error(w, fmt.Sprintf("no active tunnel for '%s' — is the client connected?", subdomain), http.StatusBadGateway)
		return
	}

	// HTTP upgrade requests (WebSocket, HTTP/2 cleartext upgrade, etc.) need raw
	// byte forwarding, not request/response semantics. After "101 Switching
	// Protocols" the connection is application-defined bytes — io.Copy in both
	// directions until either end closes.
	if isUpgradeRequest(req) {
		r.handleUpgrade(w, req, sess, subdomain, start)
		return
	}

	log.Debug().Str("subdomain", subdomain).Str("session", sess.ID).Msg("session found, opening stream")

	// Open a bidirectional stream for this request.
	st, err := sess.openStream()
	if err != nil {
		log.Error().Err(err).Msg("openStream failed")
		http.Error(w, "stream allocation failed", http.StatusInternalServerError)
		return
	}
	defer sess.CloseStream(st.id)
	log.Debug().Str("stream", st.id).Msg("stream opened, writing request")

	// Write the serialised HTTP request into the request pipe.
	// pumpRequest (already running) will read from the other end and forward
	// to the client as MsgData frames.
	//
	// We force `Connection: close` on the upstream request so the local
	// service closes the TCP connection after sending the response. Without
	// this, keep-alive servers leave the connection open and the client's
	// response-read loop has no clean EOF signal — it would block until its
	// 120s read deadline fires, even though the response is fully received.
	req.Header.Set("Connection", "close")
	req.Close = true
	go func() {
		defer st.reqW.Close() // signals pumpRequest → EOF → sends MsgReqDone
		if err := req.Write(st.reqW); err != nil {
			log.Debug().Err(err).Str("stream", st.id).Msg("writing request to pipe")
		}
	}()

	// Block reading the HTTP response from the response pipe.
	// respW is written by WriteRespData when MsgData frames arrive from client.
	resp, err := http.ReadResponse(bufio.NewReader(st.respR), req)
	if err != nil {
		log.Debug().Err(err).Str("stream", st.id).Msg("reading response from tunnel")
		http.Error(w, "tunnel error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward response headers and status to the browser.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	written, _ := io.Copy(w, resp.Body)

	// Log to DB for the admin inspector.
	_ = r.db.LogRequest(&store.RequestLog{
		ID:           uuid.New().String(),
		TunnelID:     sess.TunnelID,
		Method:       req.Method,
		Path:         req.URL.RequestURI(),
		StatusCode:   resp.StatusCode,
		DurationMs:   time.Since(start).Milliseconds(),
		ResponseSize: written,
	})

	log.Debug().
		Str("subdomain", subdomain).
		Str("method", req.Method).
		Str("path", req.URL.Path).
		Int("status", resp.StatusCode).
		Int64("ms", time.Since(start).Milliseconds()).
		Msg("proxied request")
}

// isUpgradeRequest reports whether req carries an HTTP/1.1 Upgrade — used for
// WebSocket and other protocols layered on top of HTTP. We compare both
// `Connection: upgrade` and a non-empty `Upgrade` header per RFC 7230 §6.7.
func isUpgradeRequest(req *http.Request) bool {
	if req.Header.Get("Upgrade") == "" {
		return false
	}
	for _, v := range req.Header.Values("Connection") {
		if strings.EqualFold(strings.TrimSpace(v), "upgrade") ||
			strings.Contains(strings.ToLower(v), "upgrade") {
			return true
		}
	}
	return false
}

// handleUpgrade tunnels an HTTP upgrade (WebSocket / etc.) end-to-end. After
// the local service emits "101 Switching Protocols", the wire becomes
// opaque bytes — we hijack the inbound TCP conn and bridge it to a TCP-style
// stream so neither side parses headers further.
//
// Flow:
//  1. Open a TCP-style stream to the client (MsgOpenTCP — no MsgReqDone).
//  2. Serialise and forward the original HTTP upgrade request.
//  3. Hijack the inbound conn from net/http.
//  4. io.Copy in both directions: inbound ↔ stream pipes.
func (r *Registry) handleUpgrade(w http.ResponseWriter, req *http.Request, sess *Session, subdomain string, start time.Time) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "tunnel error: upgrade not supported by transport", http.StatusInternalServerError)
		return
	}

	// Open a TCP-style stream — fully bidirectional, no req/resp boundary.
	st, err := sess.openTCPStream()
	if err != nil {
		log.Error().Err(err).Msg("openTCPStream failed (upgrade)")
		http.Error(w, "stream allocation failed", http.StatusInternalServerError)
		return
	}

	// Hijack BEFORE writing anything else — once hijacked, net/http won't
	// touch the conn again. Any error after this must close the conn directly.
	conn, brw, err := hijacker.Hijack()
	if err != nil {
		log.Error().Err(err).Msg("hijack failed")
		sess.CloseStream(st.id)
		return
	}
	defer conn.Close() //nolint:errcheck

	// Serialise the original HTTP request into the stream so the client can
	// replay it to the local service. req.Write encodes start-line + headers
	// + body on the wire.
	if err := req.Write(st.reqW); err != nil {
		log.Debug().Err(err).Str("stream", st.id).Msg("writing upgrade request")
		sess.CloseStream(st.id)
		return
	}

	// Drain anything the client buffered (rare, but be safe — there may be
	// pipelined bytes after the Upgrade request line).
	if n := brw.Reader.Buffered(); n > 0 {
		buf, _ := brw.Reader.Peek(n)
		_, _ = st.reqW.Write(buf)
		_, _ = brw.Reader.Discard(n)
	}

	// Bridge: inbound conn → stream req pipe (will be encoded as MsgData by pumpTCP).
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = io.Copy(st.reqW, conn)
		st.reqW.Close() //nolint:errcheck
	}()

	// stream resp pipe → inbound conn (MsgData frames from client land here).
	written, _ := io.Copy(conn, st.respR)
	<-pumpDone
	sess.CloseStream(st.id)

	// We don't see HTTP status codes after Hijack; mark as 101 for log shape.
	_ = r.db.LogRequest(&store.RequestLog{
		ID:           uuid.New().String(),
		TunnelID:     sess.TunnelID,
		Method:       req.Method,
		Path:         req.URL.RequestURI(),
		StatusCode:   http.StatusSwitchingProtocols,
		DurationMs:   time.Since(start).Milliseconds(),
		ResponseSize: written,
	})

	log.Debug().
		Str("subdomain", subdomain).
		Str("method", req.Method).
		Str("path", req.URL.Path).
		Str("upgrade", req.Header.Get("Upgrade")).
		Int64("ms", time.Since(start).Milliseconds()).
		Int64("bytes", written).
		Msg("proxied upgrade")
}

// extractSubdomain parses the Host header and returns the leftmost label if it
// sits directly under baseDomain. Returns "" for non-matching hosts.
func extractSubdomain(host, baseDomain string) string {
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	suffix := "." + baseDomain
	if !strings.HasSuffix(host, suffix) {
		return ""
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") {
		return ""
	}
	return sub
}

// ── Random subdomain ──────────────────────────────────────────────────────────

var (
	adjectives = []string{
		"autumn", "brave", "calm", "daring", "eager", "fancy", "gentle",
		"happy", "icy", "jolly", "kind", "lively", "misty", "neat",
		"proud", "rapid", "shiny", "tall", "unique", "vivid", "wild", "zany",
	}
	nouns = []string{
		"river", "mountain", "forest", "ocean", "breeze", "stone",
		"cloud", "valley", "creek", "ridge", "meadow", "harbor",
		"canyon", "delta", "dune", "fjord", "glade", "plain",
	}
	rng   = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	rngMu sync.Mutex
)

func randomSubdomain() string {
	rngMu.Lock()
	adj := adjectives[rng.Intn(len(adjectives))]
	noun := nouns[rng.Intn(len(nouns))]
	rngMu.Unlock()
	return fmt.Sprintf("%s-%s", adj, noun)
}
