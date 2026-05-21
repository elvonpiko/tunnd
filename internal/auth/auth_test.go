package auth_test

import (
	"strings"
	"testing"

	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ── CreateToken ───────────────────────────────────────────────────────────────

func TestCreateToken_ReturnsTokenWithHvbPrefix(t *testing.T) {
	svc := auth.New(openTestDB(t))
	tok, err := svc.CreateToken("test-label", 0)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if !strings.HasPrefix(tok.Value, "tnnd_") {
		t.Errorf("token value %q should start with tnnd_", tok.Value)
	}
	if tok.ID == "" {
		t.Error("expected non-empty token ID")
	}
	if tok.Label != "test-label" {
		t.Errorf("label = %q, want %q", tok.Label, "test-label")
	}
	if !tok.Enabled {
		t.Error("new token should be enabled")
	}
}

func TestCreateToken_TokensAreUnique(t *testing.T) {
	svc := auth.New(openTestDB(t))
	a, _ := svc.CreateToken("a", 0)
	b, _ := svc.CreateToken("b", 0)
	if a.Value == b.Value {
		t.Error("expected distinct token values")
	}
	if a.ID == b.ID {
		t.Error("expected distinct token IDs")
	}
}

// ── ValidateToken ─────────────────────────────────────────────────────────────

func TestValidateToken_AcceptsValidToken(t *testing.T) {
	svc := auth.New(openTestDB(t))
	created, _ := svc.CreateToken("valid", 0)

	got, err := svc.ValidateToken(created.Value)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got == nil {
		t.Fatal("expected token, got nil")
	}
	if got.ID != created.ID {
		t.Errorf("token ID mismatch: got %q, want %q", got.ID, created.ID)
	}
}

func TestValidateToken_RejectsUnknownToken(t *testing.T) {
	svc := auth.New(openTestDB(t))
	_, err := svc.ValidateToken("tnnd_doesnotexist")
	if err == nil {
		t.Fatal("expected error for unknown token, got nil")
	}
}

func TestValidateToken_RejectsEmptyToken(t *testing.T) {
	svc := auth.New(openTestDB(t))
	_, err := svc.ValidateToken("")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
}

// ── RevokeToken ───────────────────────────────────────────────────────────────

func TestRevokeToken_DisablesToken(t *testing.T) {
	svc := auth.New(openTestDB(t))
	tok, _ := svc.CreateToken("revoke-me", 0)

	if err := svc.RevokeToken(tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Revoked token must not validate
	_, err := svc.ValidateToken(tok.Value)
	if err == nil {
		t.Fatal("expected error validating revoked token, got nil")
	}
}

// ── ListTokens ────────────────────────────────────────────────────────────────

func TestListTokens_ReturnsAllCreated(t *testing.T) {
	svc := auth.New(openTestDB(t))

	for _, label := range []string{"alpha", "beta", "gamma"} {
		svc.CreateToken(label, 0)
	}

	tokens, err := svc.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 3 {
		t.Errorf("len = %d, want 3", len(tokens))
	}
}
