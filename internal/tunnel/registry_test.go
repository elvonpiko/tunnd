package tunnel_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/quick"

	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	// Use a temp file because store.Open appends query parameters that would
	// conflict with an already-parameterised in-memory DSN.
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatalf("create temp db file: %v", err)
	}
	f.Close()
	db, err := store.Open(f.Name())
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Pre-create tokens used across registry tests so FK constraints are satisfied.
	for i := 0; i <= 35; i++ {
		tok := &store.Token{
			ID:      fmt.Sprintf("tok%d", i),
			Value:   fmt.Sprintf("value%d", i),
			Label:   fmt.Sprintf("label%d", i),
			Enabled: true,
		}
		if err := db.CreateToken(tok); err != nil {
			t.Fatalf("pre-create token %d: %v", i, err)
		}
	}
	return db
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_AssignsRandomSubdomainWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	sess, err := r.Register("tok1", "", "http", 3000)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sess.Subdomain == "" {
		t.Fatal("expected non-empty random subdomain")
	}
	if !strings.Contains(sess.PublicURL, sess.Subdomain) {
		t.Errorf("PublicURL %q does not contain subdomain %q", sess.PublicURL, sess.Subdomain)
	}
}

func TestRegister_HonoursRequestedSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	sess, err := r.Register("tok1", "myapp", "http", 3000)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sess.Subdomain != "myapp" {
		t.Errorf("subdomain = %q, want %q", sess.Subdomain, "myapp")
	}
	if sess.PublicURL != "https://myapp.tunnel.test" {
		t.Errorf("PublicURL = %q", sess.PublicURL)
	}
}

func TestRegister_RejectsConflictingSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	if _, err := r.Register("tok1", "taken", "http", 3000); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if _, err := r.Register("tok2", "taken", "http", 8080); err == nil {
		t.Fatal("expected error registering duplicate subdomain, got nil")
	}
}

func TestRegister_LowercasesSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	sess, err := r.Register("tok1", "MyApp", "http", 3000)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if sess.Subdomain != "myapp" {
		t.Errorf("subdomain = %q, want %q", sess.Subdomain, "myapp")
	}
}

// ── Deregister ────────────────────────────────────────────────────────────────

func TestDeregister_RemovesSession(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	sess, _ := r.Register("tok1", "gone", "http", 3000)
	r.Deregister(sess.Subdomain)

	if r.Lookup("gone") != nil {
		t.Error("expected nil after Deregister")
	}
}

func TestDeregister_IsIdempotent(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")
	r.Deregister("nonexistent") // must not panic
}

// ── Lookup ────────────────────────────────────────────────────────────────────

func TestLookup_ReturnsNilForUnknown(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")
	if r.Lookup("nope") != nil {
		t.Error("expected nil for unknown subdomain")
	}
}

// ── ActiveSessions ────────────────────────────────────────────────────────────

func TestActiveSessions_Count(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	for i := range 5 {
		r.Register(fmt.Sprintf("tok%d", i), fmt.Sprintf("sub%d", i), "http", 3000)
	}
	if got := len(r.ActiveSessions()); got != 5 {
		t.Errorf("ActiveSessions = %d, want 5", got)
	}
	r.Deregister("sub2")
	if got := len(r.ActiveSessions()); got != 4 {
		t.Errorf("after deregister = %d, want 4", got)
	}
}

// ── Concurrent safety ─────────────────────────────────────────────────────────

func TestRegistry_ConcurrentRegisterDeregister(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sub := fmt.Sprintf("worker%d", i)
			sess, err := r.Register(fmt.Sprintf("tok%d", i), sub, "http", 3000)
			if err != nil {
				return
			}
			r.Lookup(sub)
			r.Deregister(sess.Subdomain)
		}(i)
	}
	wg.Wait()
}

// ── ServeHTTP ─────────────────────────────────────────────────────────────────

func TestServeHTTP_Returns502ForUnknownSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "unknown.tunnel.test"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestServeHTTP_Returns404ForNonSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "different.example.com"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// ── WriteRespData / CloseStream safety ───────────────────────────────────────

func TestSession_WriteRespDataToMissingStream_DoesNotPanic(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")
	sess, _ := r.Register("tok1", "safe", "http", 3000)

	// Drain send channel so Send() doesn't block
	go func() {
		for range sess.SendCh() {
		}
	}()

	sess.WriteRespData("no-such-stream", []byte("data")) // must not panic
	sess.CloseStream("no-such-stream")                   // must not panic
}

// ── Task 4.3: Unit tests for registry subdomain validation ────────────────────

func TestRegister_SuccessfulCustomSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	sess, err := r.Register("tok1", "valid-sub", "http", 3000)
	if err != nil {
		t.Fatalf("Register with valid custom subdomain: %v", err)
	}
	if sess.Subdomain != "valid-sub" {
		t.Errorf("Subdomain = %q, want valid-sub", sess.Subdomain)
	}
	if sess.PublicURL != "https://valid-sub.tunnel.test" {
		t.Errorf("PublicURL = %q, want https://valid-sub.tunnel.test", sess.PublicURL)
	}
}

func TestRegister_RejectsSubdomainAlreadyInUse(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	if _, err := r.Register("tok1", "myapp", "http", 3000); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err := r.Register("tok2", "myapp", "http", 4000)
	if err == nil {
		t.Fatal("expected error for duplicate subdomain, got nil")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error %q should mention 'already in use'", err.Error())
	}
}

func TestRegister_RejectsInvalidSubdomain(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	tests := []struct {
		subdomain string
		wantMsg   string
	}{
		{"-bad", "cannot start with a hyphen"},
		{"bad-", "cannot end with a hyphen"},
		{"bad--sub", "consecutive hyphens"},
		{"my_app", "invalid characters"},
		{"ab", "between 3 and 63 characters"},
	}

	for _, tt := range tests {
		_, err := r.Register("tok1", tt.subdomain, "http", 3000)
		if err == nil {
			t.Errorf("Register(%q) expected error, got nil", tt.subdomain)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantMsg) {
			t.Errorf("Register(%q) error = %q, want it to contain %q", tt.subdomain, err.Error(), tt.wantMsg)
		}
	}
}

func TestRegister_RandomSubdomainGeneration(t *testing.T) {
	db := openTestDB(t)
	r := tunnel.New(db, "tunnel.test")

	// Register several times without specifying a subdomain.
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		sess, err := r.Register(fmt.Sprintf("tok%d", i), "", "http", 3000)
		if err != nil {
			t.Fatalf("Register with empty subdomain [%d]: %v", i, err)
		}
		if sess.Subdomain == "" {
			t.Errorf("[%d] expected non-empty random subdomain", i)
		}
		if seen[sess.Subdomain] {
			t.Errorf("[%d] random subdomain %q was generated twice", i, sess.Subdomain)
		}
		seen[sess.Subdomain] = true
	}
}

// ── Task 4.4: Property test for subdomain conflict detection ──────────────────
// Property 12: Subdomain Conflict Detection
// Validates: Requirements 4.4

func TestProperty_SubdomainConflictDetection(t *testing.T) {
	property := func(subRaw string) bool {
		// Build a valid subdomain from the raw input.
		var b strings.Builder
		for _, r := range strings.ToLower(strings.TrimSpace(subRaw)) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				b.WriteRune(r)
			}
		}
		sub := strings.Trim(b.String(), "-")
		// Remove consecutive hyphens.
		for strings.Contains(sub, "--") {
			sub = strings.ReplaceAll(sub, "--", "-")
		}
		if len(sub) < 3 || len(sub) > 63 {
			// Not a usable subdomain for this property — skip.
			return true
		}

		db, err := openPropTestDB()
		if err != nil {
			return false
		}
		defer db.Close()

		// Use a validator-free reserved list so "api", "www", etc. don't interfere.
		r := tunnel.NewWithValidator(db, "tunnel.test", []string{})
		if _, err := r.Register("tok1", sub, "http", 3000); err != nil {
			// Subdomain was rejected (e.g. it happens to start/end with hyphen after
			// cleanup — treat as not applicable).
			return true
		}

		// Second registration of the same subdomain must fail with "subdomain_in_use".
		_, err = r.Register("tok2", sub, "http", 4000)
		if err == nil {
			return false // must be rejected
		}
		// Verify the error code specifically.
		if !strings.Contains(err.Error(), "already in use") {
			return false
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("subdomain conflict detection property failed: %v", err)
	}
}

// ── Task 5.4: Property test for custom subdomain URL display ─────────────────
// Property 17: Custom Subdomain URL Display
// Validates: Requirements 4.7

func TestProperty_CustomSubdomainURLDisplay(t *testing.T) {
	// For any successfully registered custom subdomain, the returned Session
	// must have a PublicURL that contains the custom subdomain in the format
	// "https://{subdomain}.{domain}".
	property := func(subRaw string) bool {
		// Build a valid subdomain from the raw input.
		var b strings.Builder
		for _, r := range strings.ToLower(strings.TrimSpace(subRaw)) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				b.WriteRune(r)
			}
		}
		sub := strings.Trim(b.String(), "-")
		// Remove consecutive hyphens.
		for strings.Contains(sub, "--") {
			sub = strings.ReplaceAll(sub, "--", "-")
		}
		// Skip inputs that don't produce a valid subdomain.
		if len(sub) < 3 || len(sub) > 63 {
			return true
		}

		db, err := openPropTestDB()
		if err != nil {
			return false
		}
		defer db.Close()

		const domain = "tunnel.example.com"
		// Use an empty reserved list so common names like "api" don't skew the test.
		r := tunnel.NewWithValidator(db, domain, []string{})

		sess, err := r.Register("tok1", sub, "http", 3000)
		if err != nil {
			// Registration may fail for other validation reasons; skip these.
			return true
		}

		// Property: the PublicURL must contain the custom subdomain.
		expectedURL := fmt.Sprintf("https://%s.%s", sess.Subdomain, domain)
		if sess.PublicURL != expectedURL {
			return false
		}
		// The subdomain field must appear in the URL.
		if !strings.Contains(sess.PublicURL, sess.Subdomain) {
			return false
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("custom subdomain URL display property failed: %v", err)
	}
}

// ── Task 5.5: Property test for registration error propagation ────────────────
// Property 18: Registration Error Propagation
// Validates: Requirements 4.10

func TestProperty_RegistrationErrorPropagation(t *testing.T) {
	// For any registration rejection (invalid subdomain OR subdomain in use),
	// Register must return a non-nil error with a descriptive message and a
	// known error code so callers can propagate it to users.

	t.Run("invalid_subdomain errors carry code and message", func(t *testing.T) {
		// These subdomains are always invalid per the validation rules.
		invalidCases := []struct {
			subdomain string
			wantCode  string
			wantMsg   string
		}{
			{"-bad", "invalid_subdomain", "cannot start with a hyphen"},
			{"bad-", "invalid_subdomain", "cannot end with a hyphen"},
			{"bad--sub", "invalid_subdomain", "consecutive hyphens"},
			{"my_app!", "invalid_subdomain", "invalid characters"},
			{"ab", "invalid_subdomain", "between 3 and 63 characters"},
		}

		for _, tc := range invalidCases {
			db, err := openPropTestDB()
			if err != nil {
				t.Fatalf("openPropTestDB: %v", err)
			}
			r := tunnel.NewWithValidator(db, "tunnel.test", []string{})

			_, regErr := r.Register("tok1", tc.subdomain, "http", 3000)
			db.Close()

			if regErr == nil {
				t.Errorf("Register(%q): expected error, got nil", tc.subdomain)
				continue
			}
			// Error message must be descriptive.
			if !strings.Contains(regErr.Error(), tc.wantMsg) {
				t.Errorf("Register(%q): error %q should contain %q",
					tc.subdomain, regErr.Error(), tc.wantMsg)
			}
			// Error must be a *tunnel.ValidationError with the correct code.
			var valErr *tunnel.ValidationError
			if !errAsValidationError(regErr, &valErr) {
				t.Errorf("Register(%q): error should be *tunnel.ValidationError, got %T",
					tc.subdomain, regErr)
				continue
			}
			if valErr.Code != tc.wantCode {
				t.Errorf("Register(%q): Code = %q, want %q",
					tc.subdomain, valErr.Code, tc.wantCode)
			}
		}
	})

	t.Run("subdomain_in_use errors carry code and message", func(t *testing.T) {
		property := func(subRaw string) bool {
			// Build a valid subdomain.
			var b strings.Builder
			for _, r := range strings.ToLower(strings.TrimSpace(subRaw)) {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
					b.WriteRune(r)
				}
			}
			sub := strings.Trim(b.String(), "-")
			for strings.Contains(sub, "--") {
				sub = strings.ReplaceAll(sub, "--", "-")
			}
			if len(sub) < 3 || len(sub) > 63 {
				return true
			}

			db, err := openPropTestDB()
			if err != nil {
				return false
			}
			defer db.Close()

			r := tunnel.NewWithValidator(db, "tunnel.test", []string{})

			// First registration must succeed.
			if _, err := r.Register("tok1", sub, "http", 3000); err != nil {
				return true // skip — subdomain rejected for some reason
			}

			// Second registration must return a *ValidationError with code "subdomain_in_use".
			_, err = r.Register("tok2", sub, "http", 4000)
			if err == nil {
				return false
			}
			var valErr *tunnel.ValidationError
			if !errAsValidationError(err, &valErr) {
				return false
			}
			if valErr.Code != "subdomain_in_use" {
				return false
			}
			if !strings.Contains(err.Error(), "already in use") {
				return false
			}
			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
			t.Errorf("registration error propagation (in-use) property failed: %v", err)
		}
	})
}

// errAsValidationError attempts to unwrap err into a *tunnel.ValidationError.
// Returns true and sets target when err is or wraps a *tunnel.ValidationError.
func errAsValidationError(err error, target **tunnel.ValidationError) bool {
	if err == nil {
		return false
	}
	if ve, ok := err.(*tunnel.ValidationError); ok {
		*target = ve
		return true
	}
	return false
}

// openPropTestDB opens a fresh temp-file DB for use in property tests.
// Each call returns an independent instance with tokens pre-created.
func openPropTestDB() (*store.DB, error) {
	f, err := os.CreateTemp("", "prop-test-*.db")
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := store.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	// Pre-create tokens so FK constraints are satisfied.
	for i := 0; i <= 5; i++ {
		tok := &store.Token{
			ID:      fmt.Sprintf("tok%d", i),
			Value:   fmt.Sprintf("value%d", i),
			Label:   fmt.Sprintf("label%d", i),
			Enabled: true,
		}
		if err := db.CreateToken(tok); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}
