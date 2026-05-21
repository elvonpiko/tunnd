package store_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/elvonpiko/tunnd/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ── Token CRUD ────────────────────────────────────────────────────────────────

func TestToken_CreateAndRetrieve(t *testing.T) {
	db := openTestDB(t)

	tok := &store.Token{
		ID:      uuid.New().String(),
		Value:   "tnnd_test123",
		Label:   "ci-runner",
		Enabled: true,
	}
	if err := db.CreateToken(tok); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := db.GetTokenByValue("tnnd_test123")
	if err != nil {
		t.Fatalf("GetTokenByValue: %v", err)
	}
	if got == nil {
		t.Fatal("expected token, got nil")
	}
	if got.Label != "ci-runner" {
		t.Errorf("label = %q, want %q", got.Label, "ci-runner")
	}
}

func TestToken_GetByValue_ReturnsNilForMissing(t *testing.T) {
	db := openTestDB(t)
	got, err := db.GetTokenByValue("tnnd_missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for missing token")
	}
}

func TestToken_Revoke(t *testing.T) {
	db := openTestDB(t)

	tok := &store.Token{ID: uuid.New().String(), Value: "tnnd_revoke", Label: "r", Enabled: true}
	db.CreateToken(tok)

	if err := db.RevokeToken(tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Revoked token should not be returned by GetTokenByValue (enabled=1 filter)
	got, err := db.GetTokenByValue("tnnd_revoke")
	if err != nil {
		t.Fatalf("GetTokenByValue: %v", err)
	}
	if got != nil {
		t.Error("expected nil after revoke")
	}
}

func TestToken_List(t *testing.T) {
	db := openTestDB(t)

	for i := range 4 {
		db.CreateToken(&store.Token{
			ID:      uuid.New().String(),
			Value:   "tnnd_list" + string(rune('a'+i)),
			Label:   "label",
			Enabled: true,
		})
	}

	tokens, err := db.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 4 {
		t.Errorf("got %d tokens, want 4", len(tokens))
	}
}

// ── Tunnel CRUD ───────────────────────────────────────────────────────────────

func TestTunnel_OpenAndClose(t *testing.T) {
	db := openTestDB(t)

	// Need a token to satisfy FK
	tok := &store.Token{ID: uuid.New().String(), Value: "tnnd_t", Label: "t", Enabled: true}
	db.CreateToken(tok)

	tr := &store.TunnelRecord{
		ID:        uuid.New().String(),
		TokenID:   tok.ID,
		Subdomain: "myapp",
		Protocol:  "http",
		PublicURL: "https://myapp.tunnel.test",
		LocalPort: 3000,
		OpenedAt:  time.Now(),
	}
	if err := db.OpenTunnel(tr); err != nil {
		t.Fatalf("OpenTunnel: %v", err)
	}

	active, err := db.ListActiveTunnels()
	if err != nil {
		t.Fatalf("ListActiveTunnels: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active tunnel, got %d", len(active))
	}

	if err := db.CloseTunnel(tr.ID); err != nil {
		t.Fatalf("CloseTunnel: %v", err)
	}

	active, _ = db.ListActiveTunnels()
	if len(active) != 0 {
		t.Errorf("expected 0 active tunnels after close, got %d", len(active))
	}
}

// ── Request log ───────────────────────────────────────────────────────────────

func TestRequestLog_InsertAndList(t *testing.T) {
	db := openTestDB(t)

	tok := &store.Token{ID: uuid.New().String(), Value: "tnnd_rl", Label: "l", Enabled: true}
	db.CreateToken(tok)

	tunnelID := uuid.New().String()
	db.OpenTunnel(&store.TunnelRecord{
		ID: tunnelID, TokenID: tok.ID, Subdomain: "log", Protocol: "http",
		PublicURL: "https://log.tunnel.test", LocalPort: 8080, OpenedAt: time.Now(),
	})

	for i := range 5 {
		db.LogRequest(&store.RequestLog{
			ID:         uuid.New().String(),
			TunnelID:   tunnelID,
			Method:     "GET",
			Path:       "/path/" + string(rune('a'+i)),
			StatusCode: 200,
			DurationMs: int64(i * 10),
		})
	}

	logs, err := db.ListRequests(tunnelID, 10)
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(logs) != 5 {
		t.Errorf("got %d request logs, want 5", len(logs))
	}
}

func TestRequestLog_LimitIsRespected(t *testing.T) {
	db := openTestDB(t)

	tok := &store.Token{ID: uuid.New().String(), Value: "tnnd_lim", Label: "l", Enabled: true}
	db.CreateToken(tok)
	tunnelID := uuid.New().String()
	db.OpenTunnel(&store.TunnelRecord{
		ID: tunnelID, TokenID: tok.ID, Subdomain: "lim", Protocol: "http",
		PublicURL: "https://lim.tunnel.test", LocalPort: 8080, OpenedAt: time.Now(),
	})

	for range 20 {
		db.LogRequest(&store.RequestLog{
			ID: uuid.New().String(), TunnelID: tunnelID,
			Method: "POST", Path: "/x", StatusCode: 201,
		})
	}

	logs, _ := db.ListRequests(tunnelID, 7)
	if len(logs) != 7 {
		t.Errorf("got %d logs with limit 7, want 7", len(logs))
	}
}
