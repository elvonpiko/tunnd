// Package store manages all persistent state via SQLite.
// Uses modernc.org/sqlite (pure-Go, CGO-free — runs on any Linux without libc dependency).
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection and exposes typed methods.
type DB struct {
	sql *sql.DB
}

// Token is an auth token record.
type Token struct {
	ID         string
	Value      string
	Label      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	MaxTunnels int
	Enabled    bool
}

// TunnelRecord is a historical tunnel entry.
type TunnelRecord struct {
	ID        string     `json:"id"`
	TokenID   string     `json:"token_id"`
	Subdomain string     `json:"subdomain"`
	Protocol  string     `json:"protocol"`
	PublicURL string     `json:"public_url"`
	LocalPort int        `json:"local_port"`
	OpenedAt  time.Time  `json:"opened_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// RequestLog is a single proxied request entry (for the inspector).
type RequestLog struct {
	ID           string    `json:"id"`
	TunnelID     string    `json:"tunnel_id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	StatusCode   int       `json:"status_code"`
	DurationMs   int64     `json:"duration_ms"`
	RequestSize  int64     `json:"request_size"`
	ResponseSize int64     `json:"response_size"`
	CreatedAt    time.Time `json:"created_at"`
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*DB, error) {
	// Build the DSN: append WAL and foreign-key pragmas using the correct
	// separator ('?' for the first parameter, '&' if query params already exist).
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	dsn := path + sep + "_journal_mode=WAL&_foreign_keys=on"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite: single writer
	db := &DB{sql: sqlDB}
	return db, db.migrate()
}

// Close closes the underlying database.
func (db *DB) Close() error { return db.sql.Close() }

// ── Migrations ────────────────────────────────────────────────────────────────

func (db *DB) migrate() error {
	_, err := db.sql.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY
		);

		CREATE TABLE IF NOT EXISTS tokens (
			id           TEXT PRIMARY KEY,
			value        TEXT UNIQUE NOT NULL,
			label        TEXT NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME,
			max_tunnels  INTEGER NOT NULL DEFAULT 0,
			enabled      INTEGER NOT NULL DEFAULT 1
		);

		CREATE TABLE IF NOT EXISTS tunnels (
			id          TEXT PRIMARY KEY,
			token_id    TEXT NOT NULL REFERENCES tokens(id),
			subdomain   TEXT NOT NULL,
			protocol    TEXT NOT NULL DEFAULT 'http',
			public_url  TEXT NOT NULL,
			local_port  INTEGER NOT NULL DEFAULT 0,
			opened_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			closed_at   DATETIME
		);

		CREATE INDEX IF NOT EXISTS idx_tunnels_token ON tunnels(token_id);
		CREATE INDEX IF NOT EXISTS idx_tunnels_subdomain ON tunnels(subdomain);

		CREATE TABLE IF NOT EXISTS request_logs (
			id            TEXT PRIMARY KEY,
			tunnel_id     TEXT NOT NULL,
			method        TEXT NOT NULL,
			path          TEXT NOT NULL,
			status_code   INTEGER NOT NULL DEFAULT 0,
			duration_ms   INTEGER NOT NULL DEFAULT 0,
			request_size  INTEGER NOT NULL DEFAULT 0,
			response_size INTEGER NOT NULL DEFAULT 0,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_request_logs_tunnel ON request_logs(tunnel_id);

		-- Key-value store for server settings (admin password hash, etc.)
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

// ── Token operations ──────────────────────────────────────────────────────────

// CreateToken inserts a new auth token.
func (db *DB) CreateToken(t *Token) error {
	_, err := db.sql.Exec(
		`INSERT INTO tokens (id, value, label, max_tunnels, enabled) VALUES (?, ?, ?, ?, ?)`,
		t.ID, t.Value, t.Label, t.MaxTunnels, boolInt(t.Enabled),
	)
	return err
}

// GetTokenByValue looks up a token by its secret value.
func (db *DB) GetTokenByValue(value string) (*Token, error) {
	row := db.sql.QueryRow(
		`SELECT id, value, label, created_at, last_used_at, max_tunnels, enabled
		 FROM tokens WHERE value = ? AND enabled = 1`, value,
	)
	return scanToken(row)
}

// GetTokenByID looks up a token by its ID.
func (db *DB) GetTokenByID(id string) (*Token, error) {
	row := db.sql.QueryRow(
		`SELECT id, value, label, created_at, last_used_at, max_tunnels, enabled
		 FROM tokens WHERE id = ?`, id,
	)
	return scanToken(row)
}

// ListTokens returns all tokens ordered by creation date.
func (db *DB) ListTokens() ([]*Token, error) {
	rows, err := db.sql.Query(
		`SELECT id, value, label, created_at, last_used_at, max_tunnels, enabled
		 FROM tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// TouchToken updates last_used_at for a token.
func (db *DB) TouchToken(id string) error {
	_, err := db.sql.Exec(
		`UPDATE tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}

// RevokeToken disables a token by ID.
func (db *DB) RevokeToken(id string) error {
	_, err := db.sql.Exec(`UPDATE tokens SET enabled = 0 WHERE id = ?`, id)
	return err
}

// DeleteToken hard-deletes a token.
func (db *DB) DeleteToken(id string) error {
	_, err := db.sql.Exec(`DELETE FROM tokens WHERE id = ?`, id)
	return err
}

// ── Tunnel operations ──────────────────────────────────────────────────────────

// OpenTunnel inserts a new active tunnel record.
func (db *DB) OpenTunnel(t *TunnelRecord) error {
	_, err := db.sql.Exec(
		`INSERT INTO tunnels (id, token_id, subdomain, protocol, public_url, local_port)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.TokenID, t.Subdomain, t.Protocol, t.PublicURL, t.LocalPort,
	)
	return err
}

// CloseTunnel marks a tunnel as closed.
func (db *DB) CloseTunnel(id string) error {
	_, err := db.sql.Exec(
		`UPDATE tunnels SET closed_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}

// ListActiveTunnels returns tunnels with no closed_at.
func (db *DB) ListActiveTunnels() ([]*TunnelRecord, error) {
	return db.queryTunnels(`WHERE closed_at IS NULL ORDER BY opened_at DESC`)
}

// ListTunnels returns all tunnels (paginated by limit/offset).
func (db *DB) ListTunnels(limit, offset int) ([]*TunnelRecord, error) {
	return db.queryTunnels(`ORDER BY opened_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

// CountTunnels returns the total number of tunnel rows. Used by the
// admin dashboard to compute pagination totals without fetching every
// row.
func (db *DB) CountTunnels() (int, error) {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM tunnels`).Scan(&n)
	return n, err
}

func (db *DB) queryTunnels(where string, args ...any) ([]*TunnelRecord, error) {
	rows, err := db.sql.Query(
		`SELECT id, token_id, subdomain, protocol, public_url, local_port, opened_at, closed_at
		 FROM tunnels `+where, args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnels := make([]*TunnelRecord, 0)
	for rows.Next() {
		var t TunnelRecord
		if err := rows.Scan(&t.ID, &t.TokenID, &t.Subdomain, &t.Protocol,
			&t.PublicURL, &t.LocalPort, &t.OpenedAt, &t.ClosedAt); err != nil {
			return nil, err
		}
		tunnels = append(tunnels, &t)
	}
	return tunnels, rows.Err()
}

// ── Request log operations ────────────────────────────────────────────────────

// LogRequest inserts a request log entry.
func (db *DB) LogRequest(r *RequestLog) error {
	_, err := db.sql.Exec(
		`INSERT INTO request_logs (id, tunnel_id, method, path, status_code, duration_ms, request_size, response_size)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TunnelID, r.Method, r.Path, r.StatusCode,
		r.DurationMs, r.RequestSize, r.ResponseSize,
	)
	return err
}

// ListRequests returns recent request logs for a tunnel.
func (db *DB) ListRequests(tunnelID string, limit int) ([]*RequestLog, error) {
	rows, err := db.sql.Query(
		`SELECT id, tunnel_id, method, path, status_code, duration_ms,
		        request_size, response_size, created_at
		 FROM request_logs WHERE tunnel_id = ? ORDER BY created_at DESC LIMIT ?`,
		tunnelID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]*RequestLog, 0)
	for rows.Next() {
		var l RequestLog
		if err := rows.Scan(&l.ID, &l.TunnelID, &l.Method, &l.Path, &l.StatusCode,
			&l.DurationMs, &l.RequestSize, &l.ResponseSize, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// ── Settings operations ───────────────────────────────────────────────────────

// GetSetting retrieves a value by key. Returns ("", nil) if not found.
func (db *DB) GetSetting(key string) (string, error) {
	var val string
	err := db.sql.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SetSetting upserts a key-value pair.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.sql.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanToken(row scanner) (*Token, error) {
	var t Token
	var enabled int
	err := row.Scan(&t.ID, &t.Value, &t.Label, &t.CreatedAt, &t.LastUsedAt, &t.MaxTunnels, &enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	return &t, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
