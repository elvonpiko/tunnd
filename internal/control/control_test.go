package control

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
	"github.com/elvonpiko/tunnd/pkg/proto"
)

func newTestHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "control-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	db, err := store.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authSvc := auth.New(db)
	registry := tunnel.New(db, "tunnd.example")
	h := New(authSvc, registry, "tunnd.example")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, srv
}

// dialRegister opens a WebSocket, sends a register frame, and reports whether
// the server accepted it: nil = registered, non-nil = the handshake error
// (auth failure, invalid subdomain, rate limit, …).
func dialRegister(t *testing.T, srv *httptest.Server, token string) error {
	t.Helper()
	url := "ws" + srv.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		// Pre-upgrade refusal (e.g. over-budget 429) fails the dial itself.
		return err
	}
	defer conn.Close()
	frame, err := proto.EncodeJSON(proto.MsgRegister, proto.RegisterPayload{
		Token:     token,
		Subdomain: "ratelimit-test",
		Protocol:  "http",
		LocalPort: 3000,
	})
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return err
	}
	// Read the reply: 'registered' on success, an error frame on rejection.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	env, err := proto.DecodeJSON(raw)
	if err != nil {
		return err
	}
	switch env.Type {
	case proto.MsgRegistered:
		return nil
	case proto.MsgError:
		var ep proto.ErrorPayload
		if err := proto.DecodePayload(env, &ep); err != nil {
			return err
		}
		return fmt.Errorf("registration rejected [%s]: %s", ep.Code, ep.Message)
	default:
		return fmt.Errorf("unexpected reply frame: %s", env.Type)
	}
}

// TestHandshake_RateLimitedAfterFailures verifies that repeated failed
// registrations (e.g. token guessing) from one source get cut off by the
// pre-upgrade 429, before a WebSocket is even opened.
func TestHandshake_RateLimitedAfterFailures(t *testing.T) {
	_, srv := newTestHandler(t)
	url := "ws" + srv.URL[len("http"):]

	// Burn the budget with bad-token registrations.
	for i := 0; i < registerRateMax; i++ {
		if err := dialRegister(t, srv, "tnnd_invalid_token_"+string(rune('a'+i))); err == nil {
			t.Fatalf("attempt %d: expected a failed handshake, got success", i+1)
		}
	}

	// Over budget: the next request is refused with 429 BEFORE the upgrade —
	// even a plain HTTP GET is enough to observe it.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("plain GET after budget burn: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("over-budget request: status=%d, want 429", resp.StatusCode)
	}

	// A WebSocket dial is also refused pre-upgrade (the 429 surfaces as a
	// handshake error for the dialer).
	if _, _, err := websocket.DefaultDialer.Dial(url, nil); err == nil {
		t.Fatal("over-budget WS dial should be refused with 429")
	}
}

// TestHandshake_SuccessResetsBudget verifies that successful registrations
// clear the failure budget — reconnecting healthy clients never accumulate
// toward the limit.
func TestHandshake_SuccessResetsBudget(t *testing.T) {
	h, srv := newTestHandler(t)
	tok, err := h.auth.CreateToken("limiter-test", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	// Interleave failures and successes; each success clears the strikes.
	for i := 0; i < registerRateMax+2; i++ {
		if err := dialRegister(t, srv, "tnnd_invalid_token"); err == nil {
			t.Fatalf("failure round %d: expected handshake failure", i+1)
		}
		if err := dialRegister(t, srv, tok.Value); err != nil {
			t.Fatalf("success round %d: expected handshake success, got %v", i+1, err)
		}
	}
}
