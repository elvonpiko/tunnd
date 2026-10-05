// Password hashing helpers for the admin dashboard.
//
// Passwords set via the bootstrap flow (or the change-password endpoint) are
// stored as bcrypt hashes. Values stored by ≤ v0.2.1 are plaintext; they are
// verified with a constant-time compare and transparently upgraded to bcrypt
// on the first successful login, so operators get the hardening without a
// migration step.
package admin

import (
	"crypto/subtle"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is bcrypt's cost factor. 10 ≈ 50–100 ms on a small VPS. Login is
// rare (single operator), and the login rate limiter is the primary
// brute-force defense — cost 10 keeps interactive logins snappy.
const bcryptCost = 10

// maxPasswordLen is bcrypt's 72-byte input limit. bcrypt silently truncates
// beyond it, so we reject longer passwords explicitly instead.
const maxPasswordLen = 72

// minPasswordLen matches the bootstrap page's minimum (12 chars).
const minPasswordLen = 12

// hashPassword returns the bcrypt hash of a new admin password.
func hashPassword(plain string) (string, error) {
	if len(plain) > maxPasswordLen {
		return "", fmt.Errorf("password must be at most %d bytes", maxPasswordLen)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// isBcryptHash reports whether a stored value is a bcrypt hash (as opposed
// to a legacy plaintext password written by tunnd ≤ v0.2.1).
func isBcryptHash(stored string) bool {
	return strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$")
}

// verifyPassword checks a candidate password against the stored secret,
// handling both bcrypt hashes (current) and legacy plaintext values.
//
// needsRehash reports that the stored value is a legacy plaintext and should
// be upgraded to bcrypt on successful verification.
func verifyPassword(stored, given string) (ok bool, needsRehash bool) {
	if stored == "" {
		return false, false
	}
	if isBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(given)) == nil, false
	}
	// Legacy plaintext: constant-time compare (same shape as the pre-1.0
	// login check), then upgrade to bcrypt on match.
	if len(given) == len(stored) &&
		subtle.ConstantTimeCompare([]byte(given), []byte(stored)) == 1 {
		return true, true
	}
	return false, false
}
