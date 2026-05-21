// Package auth handles token-based authentication for the control plane.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/elvonpiko/tunnd/internal/store"
)

// Service provides authentication operations.
type Service struct {
	db *store.DB
}

// New returns a new auth Service.
func New(db *store.DB) *Service {
	return &Service{db: db}
}

// ValidateToken checks a token value and returns the token record if valid.
// Returns nil, nil if the token does not exist or is disabled.
func (s *Service) ValidateToken(value string) (*store.Token, error) {
	if value == "" {
		return nil, fmt.Errorf("empty token")
	}
	tok, err := s.db.GetTokenByValue(value)
	if err != nil {
		return nil, fmt.Errorf("looking up token: %w", err)
	}
	if tok == nil {
		return nil, fmt.Errorf("invalid or revoked token")
	}
	_ = s.db.TouchToken(tok.ID)
	return tok, nil
}

// CreateToken generates a new cryptographically random token and stores it.
func (s *Service) CreateToken(label string, maxTunnels int) (*store.Token, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generating token bytes: %w", err)
	}
	tok := &store.Token{
		ID:         uuid.New().String(),
		Value:      "tnnd_" + hex.EncodeToString(raw),
		Label:      label,
		MaxTunnels: maxTunnels,
		Enabled:    true,
	}
	return tok, s.db.CreateToken(tok)
}

// RevokeToken disables a token by ID.
func (s *Service) RevokeToken(id string) error {
	return s.db.RevokeToken(id)
}

// ListTokens returns all tokens.
func (s *Service) ListTokens() ([]*store.Token, error) {
	return s.db.ListTokens()
}
