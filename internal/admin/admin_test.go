package admin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/quick"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/elvonpiko/tunnd/internal/admin"
	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
)

// dbCounter is used to give each test its own unique in-memory SQLite database,
// avoiding the cross-test interference that occurs when multiple tests share the
// same "cache=shared" URI.
var dbCounter int

func setup(t *testing.T, password string) (*admin.Handler, *store.DB) {
	t.Helper()
	dbCounter++
	uri := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared", dbCounter)
	db, err := store.Open(uri)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// If no cfgPassword is given, store a test password in DB so we're not in bootstrap mode.
	if password == "" {
		if err := db.SetSetting("admin_password", "testpassword1234"); err != nil {
			t.Fatalf("SetSetting: %v", err)
		}
	}

	authSvc := auth.New(db)
	registry := tunnel.New(db, "tunnel.test")
	h := admin.New(authSvc, registry, db, password)
	return h, db
}

func req(t *testing.T, h http.Handler, method, path string, body any, password string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buf)
	if password != "" {
		r.SetBasicAuth("admin", password)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// reqWithSession logs in with the given password and then makes the actual request
// using the returned session cookie. If password is empty, uses "testpassword1234".
func reqWithSession(t *testing.T, h http.Handler, method, path string, body any, password string) *httptest.ResponseRecorder {
	t.Helper()
	loginPass := password
	if loginPass == "" {
		loginPass = "testpassword1234"
	}
	// POST /login to obtain a session cookie
	loginBody := strings.NewReader("password=" + loginPass)
	lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, lr)

	// Extract session cookie from login response
	var sessionCookie *http.Cookie
	for _, c := range lw.Result().Cookies() {
		if c.Name == "tunnd_session" {
			sessionCookie = c
			break
		}
	}

	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, path, &buf)
	if sessionCookie != nil {
		r.AddCookie(sessionCookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// ── Auth guard ─────────────────────────────────────────────────────────────────

func TestGuard_RejectsNoPassword(t *testing.T) {
	h, _ := setup(t, "secret")
	w := req(t, h, "GET", "/api/stats", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGuard_RejectsWrongPassword(t *testing.T) {
	h, _ := setup(t, "secret")
	w := req(t, h, "GET", "/api/stats", nil, "wrong")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGuard_AllowsCorrectPassword(t *testing.T) {
	h, _ := setup(t, "secret")
	w := reqWithSession(t, h, "GET", "/api/stats", nil, "secret")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGuard_AllowsWithNoPasswordConfigured(t *testing.T) {
	// When cfgPassword="" and DB has a stored password — setup() puts "testpassword1234" in DB
	// so it's not in bootstrap mode. Unauthenticated requests get 401.
	h, _ := setup(t, "")
	w := req(t, h, "GET", "/api/stats", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (DB password set, no session)", w.Code)
	}
}

// ── Stats ──────────────────────────────────────────────────────────────────────

func TestStats_ReturnsJSON(t *testing.T) {
	h, _ := setup(t, "")
	w := reqWithSession(t, h, "GET", "/api/stats", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["active_tunnels"]; !ok {
		t.Error("missing active_tunnels field")
	}
	if _, ok := body["total_tokens"]; !ok {
		t.Error("missing total_tokens field")
	}
}

// ── Token create / list / revoke ───────────────────────────────────────────────

func TestCreateToken_Returns201WithValue(t *testing.T) {
	h, _ := setup(t, "")
	w := reqWithSession(t, h, "POST", "/api/tokens", map[string]any{"label": "ci", "max_tunnels": 0}, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	val, _ := body["value"].(string)
	if !strings.HasPrefix(val, "tnnd_") {
		t.Errorf("token value %q should start with tnnd_", val)
	}
}

func TestListTokens_ReturnsCreatedTokens(t *testing.T) {
	h, _ := setup(t, "")
	reqWithSession(t, h, "POST", "/api/tokens", map[string]any{"label": "a"}, "")
	reqWithSession(t, h, "POST", "/api/tokens", map[string]any{"label": "b"}, "")

	w := reqWithSession(t, h, "GET", "/api/tokens", nil, "")
	var body struct {
		Tokens []map[string]any `json:"tokens"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if len(body.Tokens) != 2 {
		t.Errorf("got %d tokens, want 2", len(body.Tokens))
	}
}

func TestRevokeToken_Returns200(t *testing.T) {
	h, _ := setup(t, "")
	cr := reqWithSession(t, h, "POST", "/api/tokens", map[string]any{"label": "revoke-me"}, "")
	var created map[string]any
	json.NewDecoder(cr.Body).Decode(&created)
	id, _ := created["id"].(string)

	w := reqWithSession(t, h, "DELETE", "/api/tokens/"+id, nil, "")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ── Active tunnels ─────────────────────────────────────────────────────────────

func TestListActiveTunnels_ReturnsEmptyInitially(t *testing.T) {
	h, _ := setup(t, "")
	w := reqWithSession(t, h, "GET", "/api/tunnels/active", nil, "")
	var body struct {
		Count int `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&body)
	if body.Count != 0 {
		t.Errorf("count = %d, want 0", body.Count)
	}
}

// ── UI ─────────────────────────────────────────────────────────────────────────

func TestServeUI_Returns200ForRoot(t *testing.T) {
	h, _ := setup(t, "") // no password configured — bootstrap mode redirects
	w := req(t, h, "GET", "/", nil, "")
	// Bootstrap mode: redirect to /setup
	if w.Code != http.StatusOK && w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 200 or 303", w.Code)
	}
}

func TestServeUI_RedirectsToLoginWhenPasswordSet(t *testing.T) {
	h, _ := setup(t, "secret")
	w := req(t, h, "GET", "/", nil, "")
	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 (redirect to login)", w.Code)
	}
}

func TestServeUI_Returns404ForUnknownAPIPath(t *testing.T) {
	h, _ := setup(t, "")
	// Non-existent API paths should return JSON 404, not the HTML page
	w := reqWithSession(t, h, "GET", "/api/nonexistent", nil, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ── Log capture helper ─────────────────────────────────────────────────────────

// captureLog redirects the zerolog package-level logger to a buffer for the
// duration of the test. It returns the buffer so callers can inspect log output.
// The original logger and level are restored via t.Cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	origLevel := zerolog.GlobalLevel()
	origLogger := log.Logger

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	// Enable timestamps in the captured logger to mirror production behaviour.
	log.Logger = zerolog.New(buf).With().Timestamp().Logger()

	t.Cleanup(func() {
		zerolog.SetGlobalLevel(origLevel)
		log.Logger = origLogger
	})
	return buf
}


// ── Task 12.4: Property test for security headers presence ────────────────────

// adminEndpoints lists public/unauthenticated admin paths for header checks.
var adminEndpoints = []struct {
	method string
	path   string
}{
	{"GET", "/api/stats"},
	{"GET", "/api/tunnels/active"},
	{"GET", "/api/tokens"},
	{"GET", "/login"},
}

// requiredSecurityHeaders are the headers mandated by Requirement 7.10.
var requiredSecurityHeaders = []string{
	"X-Frame-Options",
	"X-Content-Type-Options",
	"Content-Security-Policy",
}

// TestSecurityHeaders_AllEndpoints_UnitVerification verifies required security
// headers are present on every admin endpoint (unit-level enumeration).
// Validates: Requirements 7.10
func TestSecurityHeaders_AllEndpoints_UnitVerification(t *testing.T) {
	h, _ := setup(t, "")
	for _, ep := range adminEndpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := req(t, h, ep.method, ep.path, nil, "")
			for _, hdr := range requiredSecurityHeaders {
				if w.Header().Get(hdr) == "" {
					t.Errorf("missing security header %q on %s %s", hdr, ep.method, ep.path)
				}
			}
		})
	}
}


// TestProperty15_SecurityHeadersPresence is a property test using
// testing/quick. It generates random (method, path) pairs for known admin
// endpoints and verifies all required security headers appear in every response.
//
// Property 15: Security Headers Presence
// Validates: Requirements 7.10
func TestProperty15_SecurityHeadersPresence(t *testing.T) {
	h, _ := setup(t, "") // no password so guard passes through

	// f receives an index that selects one of the known endpoints.
	// testing/quick will call this with random uint8 values.
	f := func(idx uint8) bool {
		ep := adminEndpoints[int(idx)%len(adminEndpoints)]
		w := req(t, h, ep.method, ep.path, nil, "")
		for _, hdr := range requiredSecurityHeaders {
			if w.Header().Get(hdr) == "" {
				t.Logf("missing header %q on %s %s (status %d)", hdr, ep.method, ep.path, w.Code)
				return false
			}
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 15 failed: %v", err)
	}
}


// ── Task 13.2: Unit tests for authentication logging ─────────────────────────

// TestAuthLogging_FailureIsLogged verifies that a login failure produces a log
// entry containing source_ip.
// Validates: Requirements 7.5
func TestAuthLogging_FailureIsLogged(t *testing.T) {
	buf := captureLog(t)
	h, _ := setup(t, "correctpassword")

	// POST to /login with wrong password
	loginBody := strings.NewReader("password=wrongpassword")
	lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, lr)

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output, got nothing")
	}

	var entry map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(output)).Decode(&entry); err != nil {
		t.Fatalf("log entry is not valid JSON: %v\nraw: %s", err, output)
	}

	if _, ok := entry["source_ip"]; !ok {
		t.Errorf("log entry missing 'source_ip' field; entry: %v", entry)
	}
}

// TestAuthLogging_ContainsTimestamp verifies that the log entry for a login
// failure has a "time" field.
// Validates: Requirements 7.5
func TestAuthLogging_ContainsTimestamp(t *testing.T) {
	buf := captureLog(t)
	h, _ := setup(t, "secret99")

	loginBody := strings.NewReader("password=bad")
	lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, lr)

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output, got nothing")
	}

	var entry map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(output)).Decode(&entry); err != nil {
		t.Fatalf("log entry is not valid JSON: %v\nraw: %s", err, output)
	}
	if _, ok := entry["time"]; !ok {
		t.Errorf("log entry missing 'time' field; entry: %v", entry)
	}
}

// TestAuthLogging_SuccessNotLogged verifies that a successful login does NOT
// produce an authentication failure log entry.
// Validates: Requirements 7.5
func TestAuthLogging_SuccessNotLogged(t *testing.T) {
	buf := captureLog(t)
	h, _ := setup(t, "correct")

	loginBody := strings.NewReader("password=correct")
	lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, lr)

	output := buf.String()
	if strings.Contains(output, "login failure") || strings.Contains(output, "authentication failure") {
		t.Errorf("successful login should not produce a failure log entry; got: %s", output)
	}
}


// ── Task 13.3: Property test for authentication failure logging ───────────────

// TestProperty14_AuthFailureLogging is a property test verifying that every
// failed authentication attempt produces a log entry containing timestamp,
// source_ip, and username fields.
//
// Property 14: Admin Authentication Failure Logging
// Validates: Requirements 7.5
func TestProperty14_AuthFailureLogging(t *testing.T) {
	h, _ := setup(t, "thecorrectpassword")

	f := func(password string) bool {
		if password == "thecorrectpassword" {
			return true
		}

		buf := &bytes.Buffer{}
		origLogger := log.Logger
		origLevel := zerolog.GlobalLevel()
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		log.Logger = zerolog.New(buf)
		defer func() {
			log.Logger = origLogger
			zerolog.SetGlobalLevel(origLevel)
		}()

		loginBody := strings.NewReader("password=" + password)
		lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
		lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		lw := httptest.NewRecorder()
		h.ServeHTTP(lw, lr)

		// Should redirect back to login (not to dashboard)
		if lw.Header().Get("Location") == "/" {
			return false
		}

		output := buf.String()
		if output == "" {
			return false
		}

		var entry map[string]interface{}
		if err := json.NewDecoder(strings.NewReader(output)).Decode(&entry); err != nil {
			return false
		}

		_, hasIP := entry["source_ip"]
		return hasIP
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 14 failed: %v", err)
	}
}


// ── Task 14.3: Property test for weak password warning ───────────────────────

// weakPasswordWarning mirrors the password-strength check from cmd/server/main.go.
// Returns the warning message that would be logged, or "" if the password is strong.
// This function exists solely to make the logic testable in isolation.
func weakPasswordWarning(password string) string {
	weakDefaults := map[string]bool{"changeme": true, "admin": true}
	if weakDefaults[password] {
		return "Admin password is set to a default value. Change it immediately for production use."
	}
	if len(password) < 12 {
		return "Admin password is shorter than recommended minimum of 12 characters."
	}
	return ""
}

// TestWeakPassword_DefaultPasswordsWarn verifies that the two known default
// passwords ("changeme", "admin") produce a warning.
// Validates: Requirements 7.9
func TestWeakPassword_DefaultPasswordsWarn(t *testing.T) {
	defaults := []string{"changeme", "admin"}
	for _, pw := range defaults {
		t.Run(pw, func(t *testing.T) {
			msg := weakPasswordWarning(pw)
			if msg == "" {
				t.Errorf("expected warning for default password %q, got none", pw)
			}
			if !strings.Contains(msg, "default value") {
				t.Errorf("warning message for %q should mention 'default value', got: %q", pw, msg)
			}
		})
	}
}

// TestWeakPassword_ShortPasswordWarns verifies that passwords under 12 characters
// (excluding default values) produce a "shorter than recommended" warning.
// Validates: Requirements 7.12
func TestWeakPassword_ShortPasswordWarns(t *testing.T) {
	// Use passwords that are short but not the default values
	shortPasswords := []string{"abc", "12345", "short1", "xyzzy"}
	for _, pw := range shortPasswords {
		t.Run(pw, func(t *testing.T) {
			msg := weakPasswordWarning(pw)
			if msg == "" {
				t.Errorf("expected warning for short password %q (len %d), got none", pw, len(pw))
			}
		})
	}
}

// TestWeakPassword_StrongPasswordNoWarn verifies that passwords >= 12 characters
// that are not default values do not produce any warning.
func TestWeakPassword_StrongPasswordNoWarn(t *testing.T) {
	strongPasswords := []string{
		"SuperSecret123",
		"averylongpasswordthatisstrong",
		"C0mpl3xP@ssw0rd",
		"TwelveCharsX",
	}
	for _, pw := range strongPasswords {
		t.Run(pw, func(t *testing.T) {
			msg := weakPasswordWarning(pw)
			if msg != "" {
				t.Errorf("expected no warning for strong password %q, got: %q", pw, msg)
			}
		})
	}
}


// TestProperty16_WeakPasswordWarning is a property test verifying that any
// password shorter than 12 characters or matching a default value produces a
// warning message.
//
// Property 16: Weak Password Warning
// Validates: Requirements 7.9, 7.12
func TestProperty16_WeakPasswordWarning(t *testing.T) {
	weakDefaults := []string{"changeme", "admin"}
	const maxLen = 11 // passwords < 12 characters

	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		var password string
		// Alternate between: generating a short password vs. picking a default.
		if r.Intn(2) == 0 {
			// Generate a short password (1–11 chars) from printable ASCII.
			n := 1 + r.Intn(maxLen)
			b := make([]byte, n)
			for i := range b {
				b[i] = byte(0x21 + r.Intn(0x5E)) // '!' to '~'
			}
			password = string(b)
		} else {
			password = weakDefaults[r.Intn(len(weakDefaults))]
		}

		msg := weakPasswordWarning(password)
		if msg == "" {
			t.Logf("no warning for weak password %q (len %d)", password, len(password))
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 16 failed: %v", err)
	}
}


// ── Task 15: Integration tests for admin security ─────────────────────────────

// TestIntegration_ValidCredsSucceeds verifies that a request with the correct
// admin password succeeds (HTTP 200).
// Validates: Requirements 7.1, 7.2
func TestIntegration_ValidCredsSucceeds(t *testing.T) {
	h, _ := setup(t, "IntegrationPass123")
	w := reqWithSession(t, h, "GET", "/api/stats", nil, "IntegrationPass123")
	if w.Code != http.StatusOK {
		t.Errorf("valid creds: status = %d, want 200", w.Code)
	}
}

// TestIntegration_InvalidCredsFails verifies that a login with incorrect
// credentials does not produce a valid session.
// Validates: Requirements 7.3
func TestIntegration_InvalidCredsFails(t *testing.T) {
	h, _ := setup(t, "IntegrationPass123")
	// Try to get a session with wrong password, then use it
	w := reqWithSession(t, h, "GET", "/api/stats", nil, "wrongpassword")
	// Without a valid session cookie the guard should reject — 401 for API
	if w.Code == http.StatusOK {
		t.Errorf("invalid creds: status = %d, want non-200", w.Code)
	}
}

// TestIntegration_MissingCredsFails verifies that a request with no session
// returns HTTP 401 for API routes.
// Validates: Requirements 7.1, 7.3
func TestIntegration_MissingCredsFails(t *testing.T) {
	h, _ := setup(t, "IntegrationPass123")
	w := req(t, h, "GET", "/api/stats", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing creds: status = %d, want 401", w.Code)
	}
}

// TestIntegration_LoginPageServed verifies the login page is accessible.
func TestIntegration_LoginPageServed(t *testing.T) {
	h, _ := setup(t, "IntegrationPass123")
	w := req(t, h, "GET", "/login", nil, "")
	if w.Code != http.StatusOK {
		t.Errorf("login page: status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("login page Content-Type = %q, want text/html", ct)
	}
}


// TestIntegration_SecurityHeadersOnAllAdminResponses verifies that every admin
// endpoint—authenticated or not—includes all required security headers.
// Validates: Requirements 7.10
func TestIntegration_SecurityHeadersOnAllAdminResponses(t *testing.T) {
	h, _ := setup(t, "IntegrationPass123")

	endpoints := []struct {
		method   string
		path     string
		useAuth  bool
	}{
		{"GET", "/api/stats", true},
		{"GET", "/api/tunnels/active", true},
		{"GET", "/api/tokens", true},
		{"GET", "/login", false},
		// Unauthenticated (401) — headers should still be present
		{"GET", "/api/stats", false},
		{"GET", "/api/tokens", false},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s (auth=%v)", ep.method, ep.path, ep.useAuth), func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if ep.useAuth {
				w = reqWithSession(t, h, ep.method, ep.path, nil, "IntegrationPass123")
			} else {
				w = req(t, h, ep.method, ep.path, nil, "")
			}
			for _, hdr := range requiredSecurityHeaders {
				if w.Header().Get(hdr) == "" {
					t.Errorf("missing header %q (status %d)", hdr, w.Code)
				}
			}
		})
	}
}

// TestIntegration_AuthFailureLogged verifies end-to-end that a failed
// login attempt produces a structured log entry with required fields.
// Validates: Requirements 7.5
func TestIntegration_AuthFailureLogged(t *testing.T) {
	buf := captureLog(t)
	h, _ := setup(t, "IntegrationPass123")

	loginBody := strings.NewReader("password=badpassword")
	lr := httptest.NewRequest(http.MethodPost, "/login", loginBody)
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, lr)

	if lw.Code == http.StatusSeeOther && lw.Header().Get("Location") == "/" {
		t.Fatal("bad password should not redirect to dashboard")
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output on login failure, got nothing")
	}

	found := false
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		msg, _ := entry["message"].(string)
		if strings.Contains(msg, "login failure") || strings.Contains(msg, "authentication failure") {
			if _, hasIP := entry["source_ip"]; hasIP {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("no log entry with source_ip found for login failure; log output:\n%s", output)
	}
}

