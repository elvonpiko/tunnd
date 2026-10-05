// Package control implements the WebSocket control-plane server.
// Each connecting Tunnd client gets one goroutine for reading
// and uses the session's send channel for writing.
package control

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/ratelimit"
	"github.com/elvonpiko/tunnd/internal/tunnel"
	"github.com/elvonpiko/tunnd/pkg/proto"
)

const (
	writeWait      = 30 * time.Second // increased from 10s — allows for slower connections
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4 * 1024 * 1024 // 4 MiB per frame

	// Handshake failure budget: 10 failed registrations per IP per minute.
	// Legitimate clients never hit this (a successful register resets the
	// key); token-guessing spam gets a plain 429 without even a WS upgrade.
	registerRateWindow = time.Minute
	registerRateMax    = 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // auth is token-based
}

// Handler handles WebSocket upgrades on the control plane.
type Handler struct {
	auth             *auth.Service
	registry         *tunnel.Registry
	domain           string
	registerLimiter  *ratelimit.Limiter
}

// New returns a new control-plane Handler.
func New(authSvc *auth.Service, registry *tunnel.Registry, domain string) *Handler {
	return &Handler{
		auth:            authSvc,
		registry:        registry,
		domain:          domain,
		registerLimiter: ratelimit.New(registerRateWindow, registerRateMax),
	}
}

// ServeHTTP upgrades the HTTP connection to a WebSocket and drives the session.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handshake-failure throttle, keyed on the TCP peer address. Checked
	// before the upgrade so over-budget sources get a cheap 429 instead of
	// a WebSocket to spam auth attempts through.
	ipKey := peerIP(r)
	if !h.registerLimiter.Allow(ipKey) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "too many failed registration attempts — retry in a minute", http.StatusTooManyRequests)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("websocket upgrade failed")
		return
	}
	conn.SetReadLimit(maxMessageSize)

	sess, err := h.handshake(conn)
	if err != nil {
		h.registerLimiter.Strike(ipKey)
		log.Warn().Err(err).Str("remote", r.RemoteAddr).Msg("handshake failed")
		if ve, ok := err.(*tunnel.ValidationError); ok {
			sendError(conn, ve.Code, ve.Message)
		} else {
			sendError(conn, "handshake_failed", err.Error())
		}
		conn.Close()
		return
	}

	defer func() {
		// Deregister by session identity: if this session was replaced by a
		// newer registration of the same subdomain (takeover), the registry
		// already holds the new one and this is a no-op on the map.
		h.registry.DeregisterSession(sess)
		conn.Close()
		log.Info().Str("subdomain", sess.Subdomain).Msg("client disconnected")
	}()

	// A successful handshake clears this IP's failure budget — reconnect
	// storms from healthy clients never accumulate toward the limit.
	h.registerLimiter.Reset(ipKey)

	log.Info().
		Str("subdomain", sess.Subdomain).
		Str("public_url", sess.PublicURL).
		Msg("tunnel established")

	go h.writer(conn, sess)
	h.reader(conn, sess)
}

// handshake reads the first MsgRegister frame, validates the token, and
// registers a tunnel. Returns the active session on success.
func (h *Handler) handshake(conn *websocket.Conn) (*tunnel.Session, error) {
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("reading register frame: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	if proto.FrameKind(raw) != proto.FrameKindJSON {
		return nil, fmt.Errorf("expected JSON envelope, got kind 0x%02x", proto.FrameKind(raw))
	}
	env, err := proto.DecodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding envelope: %w", err)
	}
	if env.Type != proto.MsgRegister {
		return nil, fmt.Errorf("expected 'register', got '%s'", env.Type)
	}

	var reg proto.RegisterPayload
	if err := proto.DecodePayload(env, &reg); err != nil {
		return nil, fmt.Errorf("decoding register payload: %w", err)
	}

	tok, err := h.auth.ValidateToken(reg.Token)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	protocol := reg.Protocol
	if protocol != "http" && protocol != "tcp" {
		protocol = "http"
	}

	sess, err := h.registry.Register(tok.ID, reg.Subdomain, protocol, reg.LocalPort)
	if err != nil {
		return nil, err
	}

	// Echo the client's host-header policy onto the session as-is. Empty
	// string is preserved (Phase 2 treats it as the new default "rewrite"
	// at apply-time, not at storage time). UpstreamScheme stays purely
	// client-side — it does not need to be stored on the server session.
	sess.HostHeader = reg.HostHeader

	msg, err := proto.EncodeJSON(proto.MsgRegistered, proto.RegisteredPayload{
		Subdomain: sess.Subdomain,
		PublicURL: sess.PublicURL,
		TunnelID:  sess.TunnelID,
	})
	if err != nil {
		h.registry.Deregister(sess.Subdomain)
		return nil, err
	}
	sess.Send(msg)
	return sess, nil
}

// reader processes all incoming frames from the client.
//
// Frame routing:
//   - Binary data (kind 0x02) → response bytes for a stream → WriteRespData
//   - JSON envelope (kind 0x01) with MsgClose → CloseStream
//   - JSON envelope with MsgPing  → respond with MsgPong
func (h *Handler) reader(conn *websocket.Conn, sess *tunnel.Session) {
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debug().Err(err).Str("session", sess.ID).Msg("ws read error")
			}
			return
		}

		switch proto.FrameKind(raw) {
		case proto.FrameKindBinaryData:
			streamID, payload, err := proto.DecodeBinaryData(raw)
			if err != nil {
				log.Warn().Err(err).Msg("decoding binary data frame")
				continue
			}
			sess.WriteRespData(streamID, payload)
			continue

		case proto.FrameKindJSON:
			env, err := proto.DecodeJSON(raw)
			if err != nil {
				log.Warn().Err(err).Msg("decoding json frame")
				continue
			}
			switch env.Type {
			case proto.MsgClose:
				var cp proto.ClosePayload
				if err := proto.DecodePayload(env, &cp); err != nil {
					log.Warn().Err(err).Msg("decoding close payload")
					continue
				}
				sess.CloseStream(cp.StreamID)

			case proto.MsgPing:
				msg, _ := proto.EncodeJSON(proto.MsgPong, nil)
				sess.Send(msg)

			default:
				log.Debug().Str("type", string(env.Type)).Msg("unknown frame type, ignoring")
			}

		default:
			log.Warn().Int("kind", int(proto.FrameKind(raw))).Msg("unknown frame kind")
		}
	}
}

// writer drains the session send channel and writes to the WebSocket.
// When the writer exits (write error or connection close), it closes the
// underlying connection so the reader goroutine also exits and the session
// is properly deregistered.
//
// The session can also be kicked server-side (replaced by a newer registration
// of the same subdomain, or revoked). The writer then sends a close frame with
// CloseCodeSessionTakenOver so the client knows NOT to auto-reconnect.
func (h *Handler) writer(conn *websocket.Conn, sess *tunnel.Session) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// Close the connection so the reader goroutine exits and triggers Deregister.
		conn.Close()
	}()

	for {
		select {
		case msg, ok := <-sess.SendCh():
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{}) //nolint:errcheck
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				log.Debug().Err(err).Str("session", sess.ID).Msg("ws write error — closing connection")
				return
			}

		case <-sess.Kicked():
			// Tell the client why the session died, then close. Best-effort —
			// if the write fails the TCP close still reaches the client.
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage( //nolint:errcheck
				proto.CloseCodeSessionTakenOver, sess.KickReason(),
			))
			log.Info().
				Str("subdomain", sess.Subdomain).
				Str("reason", sess.KickReason()).
				Msg("session kicked — closing control WebSocket")
			return

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Debug().Err(err).Str("session", sess.ID).Msg("ws ping error — closing connection")
				return
			}
		}
	}
}

// sendError sends an error frame then closes the connection.
func sendError(conn *websocket.Conn, code, message string) {
	msg, err := proto.EncodeJSON(proto.MsgError, proto.ErrorPayload{Code: code, Message: message})
	if err != nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	conn.WriteMessage(websocket.BinaryMessage, msg) //nolint:errcheck
}

// peerIP extracts the host portion of the TCP peer address. Never trusts
// forwarded headers: an attacker on a directly exposed control port must not
// get a fresh rate-limit budget per fake X-Forwarded-For value.
func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
