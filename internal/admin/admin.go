// Package admin implements the embedded admin dashboard HTTP API + UI.
// On first run (no admin password set), the server serves a one-time
// bootstrap/setup page so the operator can set their admin password.
// After setup, authentication uses a session cookie (12-hour TTL).
package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
)

const (
	sessionCookieName = "tunnd_session"
	sessionTTL        = 12 * time.Hour
	settingAdminPass  = "admin_password"
)

// session holds a single authenticated admin session.
type session struct {
	createdAt time.Time
}

// Handler provides the admin REST API and the embedded dashboard UI.
type Handler struct {
	auth     *auth.Service
	registry *tunnel.Registry
	db       *store.DB
	// cfgPassword is the password from the config file / env var (legacy).
	// If empty, the DB-stored password is used instead.
	cfgPassword string
	mux         *http.ServeMux

	sessMu   sync.Mutex
	sessions map[string]*session
}

// New returns a configured admin Handler.
// cfgPassword may be empty — in that case the password is read from (and written to) the DB.
func New(authSvc *auth.Service, registry *tunnel.Registry, db *store.DB, cfgPassword string) *Handler {
	h := &Handler{
		auth:        authSvc,
		registry:    registry,
		db:          db,
		cfgPassword: cfgPassword,
		mux:         http.NewServeMux(),
		sessions:    make(map[string]*session),
	}
	h.registerRoutes()
	go h.reapSessions()
	return h
}

// isBootstrap returns true when no admin password has been configured yet.
func (h *Handler) isBootstrap() bool {
	if h.cfgPassword != "" {
		return false
	}
	stored, _ := h.db.GetSetting(settingAdminPass)
	return stored == ""
}

// password returns the effective admin password (cfg > DB).
func (h *Handler) password() string {
	if h.cfgPassword != "" {
		return h.cfgPassword
	}
	stored, _ := h.db.GetSetting(settingAdminPass)
	return stored
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	// Bootstrap setup (only active when no password is set)
	h.mux.Handle("GET /setup", h.securityHeaders(http.HandlerFunc(h.serveSetup)))
	h.mux.Handle("POST /setup", h.securityHeaders(http.HandlerFunc(h.handleSetup)))

	// Auth routes
	h.mux.Handle("GET /login", h.securityHeaders(http.HandlerFunc(h.serveLogin)))
	h.mux.Handle("POST /login", h.securityHeaders(http.HandlerFunc(h.handleLogin)))
	h.mux.Handle("POST /logout", h.securityHeaders(http.HandlerFunc(h.handleLogout)))

	// Protected API
	h.mux.Handle("GET /api/stats", h.securityHeaders(h.guard(h.stats)))
	h.mux.Handle("GET /api/tunnels/active", h.securityHeaders(h.guard(h.listActiveTunnels)))
	h.mux.Handle("GET /api/tunnels", h.securityHeaders(h.guard(h.listTunnels)))
	h.mux.Handle("GET /api/tunnels/{id}/requests", h.securityHeaders(h.guard(h.listRequests)))
	h.mux.Handle("GET /api/tokens", h.securityHeaders(h.guard(h.listTokens)))
	h.mux.Handle("POST /api/tokens", h.securityHeaders(h.guard(h.createToken)))
	h.mux.Handle("DELETE /api/tokens/{id}", h.securityHeaders(h.guard(h.revokeToken)))

	// Protected dashboard catch-all
	h.mux.Handle("/", h.securityHeaders(h.guard(h.serveUI)))
}

// ── Session management ─────────────────────────────────────────────────────────

func (h *Handler) newSession() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	h.sessMu.Lock()
	h.sessions[token] = &session{createdAt: time.Now()}
	h.sessMu.Unlock()
	return token, nil
}

func (h *Handler) validateSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	h.sessMu.Lock()
	sess, ok := h.sessions[cookie.Value]
	h.sessMu.Unlock()
	if !ok {
		return false
	}
	return time.Since(sess.createdAt) < sessionTTL
}

func (h *Handler) deleteSession(r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return
	}
	h.sessMu.Lock()
	delete(h.sessions, cookie.Value)
	h.sessMu.Unlock()
}

func (h *Handler) reapSessions() {
	tick := time.NewTicker(30 * time.Minute)
	defer tick.Stop()
	for range tick.C {
		now := time.Now()
		h.sessMu.Lock()
		for tok, sess := range h.sessions {
			if now.Sub(sess.createdAt) >= sessionTTL {
				delete(h.sessions, tok)
			}
		}
		h.sessMu.Unlock()
	}
}

// ── Middleware ─────────────────────────────────────────────────────────────────

func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// guard redirects to /setup if no password is configured, then checks session.
func (h *Handler) guard(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First-run: no password set at all — go to bootstrap setup
		if h.isBootstrap() {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				apiErr(w, "server not configured — visit /setup", http.StatusServiceUnavailable)
				return
			}
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}

		if !h.validateSession(r) {
			sourceIP := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				sourceIP = forwarded
			} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				sourceIP = realIP
			}
			log.Warn().
				Str("source_ip", sourceIP).
				Str("path", r.URL.Path).
				Msg("admin authentication failure")

			if strings.HasPrefix(r.URL.Path, "/api/") {
				apiErr(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next(w, r)
	})
}

// ── Bootstrap / setup handlers ────────────────────────────────────────────────

func (h *Handler) serveSetup(w http.ResponseWriter, r *http.Request) {
	// If already configured, redirect to login
	if !h.isBootstrap() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := r.URL.Query().Get("error")
	page := strings.Replace(setupHTML, "%s", htmlEscape(errMsg), 1)
	fmt.Fprint(w, page)
}

func (h *Handler) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !h.isBootstrap() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/setup?error=bad+request", http.StatusSeeOther)
		return
	}
	pass := r.FormValue("password")
	if len(pass) < 12 {
		http.Redirect(w, r, "/setup?error=Password+must+be+at+least+12+characters", http.StatusSeeOther)
		return
	}
	if err := h.db.SetSetting(settingAdminPass, pass); err != nil {
		http.Redirect(w, r, "/setup?error=Internal+error+saving+password", http.StatusSeeOther)
		return
	}
	log.Info().Msg("admin password set via bootstrap setup")

	// Immediately create a session so the user lands on the dashboard
	tok, err := h.newSession()
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ── Login / logout handlers ───────────────────────────────────────────────────

func (h *Handler) serveLogin(w http.ResponseWriter, r *http.Request) {
	if h.isBootstrap() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if h.validateSession(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := r.URL.Query().Get("error")
	page := strings.Replace(loginHTML, "%s", htmlEscape(errMsg), 1)
	fmt.Fprint(w, page)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=bad+request", http.StatusSeeOther)
		return
	}
	pass := r.FormValue("password")
	want := []byte(h.password())
	got := []byte(pass)

	ok := len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1
	if !ok {
		sourceIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			sourceIP = forwarded
		} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			sourceIP = realIP
		}
		log.Warn().
			Str("source_ip", sourceIP).
			Msg("admin login failure")
		http.Redirect(w, r, "/login?error=Invalid+password", http.StatusSeeOther)
		return
	}

	tok, err := h.newSession()
	if err != nil {
		http.Redirect(w, r, "/login?error=Internal+error", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	log.Info().Str("source_ip", r.RemoteAddr).Msg("admin login successful")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.deleteSession(r)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ── Stats ──────────────────────────────────────────────────────────────────────

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	sessions := h.registry.ActiveSessions()
	tokens, _ := h.auth.ListTokens()
	apiOK(w, map[string]any{
		"active_tunnels": len(sessions),
		"total_tokens":   len(tokens),
		"server_time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// ── Tunnel endpoints ───────────────────────────────────────────────────────────

func (h *Handler) listActiveTunnels(w http.ResponseWriter, r *http.Request) {
	sessions := h.registry.ActiveSessions()
	type view struct {
		ID        string `json:"id"`
		TunnelID  string `json:"tunnel_id"`
		Subdomain string `json:"subdomain"`
		Protocol  string `json:"protocol"`
		PublicURL string `json:"public_url"`
	}
	views := make([]view, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, view{
			ID: s.ID, TunnelID: s.TunnelID,
			Subdomain: s.Subdomain, Protocol: s.Protocol, PublicURL: s.PublicURL,
		})
	}
	apiOK(w, map[string]any{"tunnels": views, "count": len(views)})
}

func (h *Handler) listTunnels(w http.ResponseWriter, r *http.Request) {
	limit := clamp(queryInt(r, "limit", 50), 1, 200)
	offset := queryInt(r, "offset", 0)
	tunnels, err := h.db.ListTunnels(limit, offset)
	if err != nil {
		apiErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	apiOK(w, map[string]any{"tunnels": tunnels, "limit": limit, "offset": offset})
}

func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		apiErr(w, "missing tunnel id", http.StatusBadRequest)
		return
	}
	limit := clamp(queryInt(r, "limit", 100), 1, 500)
	logs, err := h.db.ListRequests(id, limit)
	if err != nil {
		apiErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	apiOK(w, map[string]any{"requests": logs})
}

// ── Token endpoints ────────────────────────────────────────────────────────────

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.auth.ListTokens()
	if err != nil {
		apiErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type view struct {
		ID         string `json:"id"`
		Label      string `json:"label"`
		ValueHint  string `json:"value_hint"`
		MaxTunnels int    `json:"max_tunnels"`
		Enabled    bool   `json:"enabled"`
	}
	views := make([]view, 0, len(tokens))
	for _, t := range tokens {
		hint := t.Value
		if len(hint) > 16 {
			hint = hint[:16] + "…"
		}
		views = append(views, view{
			ID: t.ID, Label: t.Label, ValueHint: hint,
			MaxTunnels: t.MaxTunnels, Enabled: t.Enabled,
		})
	}
	apiOK(w, map[string]any{"tokens": views})
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label      string `json:"label"`
		MaxTunnels int    `json:"max_tunnels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apiErr(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Label == "" {
		body.Label = "unnamed"
	}
	tok, err := h.auth.CreateToken(body.Label, body.MaxTunnels)
	if err != nil {
		apiErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info().Str("token_id", tok.ID).Str("label", tok.Label).Msg("token created via admin API")
	w.WriteHeader(http.StatusCreated)
	apiOK(w, map[string]any{
		"id": tok.ID, "label": tok.Label,
		"value":       tok.Value,
		"max_tunnels": tok.MaxTunnels, "enabled": tok.Enabled,
	})
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		apiErr(w, "missing token id", http.StatusBadRequest)
		return
	}
	if err := h.auth.RevokeToken(id); err != nil {
		apiErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Info().Str("token_id", id).Msg("token revoked via admin API")
	apiOK(w, map[string]any{"revoked": id})
}

// ── UI ─────────────────────────────────────────────────────────────────────────

func (h *Handler) serveUI(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		apiErr(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func apiOK(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("JSON encode")
	}
}

func apiErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

func queryInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
