// Package config loads and validates Tunnd configuration.
//
// Priority (highest → lowest):
//  1. CLI overrides (when provided to LoadClient)
//  2. Environment variables  TUNND_<KEY>
//  3. Config file            tunnd-server.yaml / tunnd.yaml
//  4. Built-in defaults
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Server holds all server-side configuration.
type Server struct {
	// Domain is the base domain for tunnels (e.g. "tunnel.example.com").
	// Tunnels are exposed as <subdomain>.<Domain>.
	// A wildcard DNS record *.tunnel.example.com → <server-ip> is required.
	Domain string `mapstructure:"domain"`

	// HTTPPort is the public port for tunnel traffic. Default 443.
	HTTPPort int `mapstructure:"http_port"`

	// AdminPort is the port for the admin dashboard + API. Default 9091.
	AdminPort int `mapstructure:"admin_port"`

	// ── TLS: pick ONE of the three options below ──────────────────────────

	// TLSEmail enables automatic Let's Encrypt certificates.
	// Port 80 must be publicly reachable for the HTTP-01 ACME challenge.
	// Leave empty to use manual certs or run in plain HTTP mode (dev).
	TLSEmail string `mapstructure:"tls_email"`

	// ACMECacheDir is where autocert stores issued certificates.
	// Defaults to ./.autocert-cache  — use a persistent path in production.
	ACMECacheDir string `mapstructure:"acme_cache_dir"`

	// TLSCertFile + TLSKeyFile — bring your own certificate (PEM format).
	// Takes priority over TLSEmail when both are set.
	// Use this when you have a wildcard cert from Certbot / your CA.
	TLSCertFile string `mapstructure:"tls_cert_file"`
	TLSKeyFile  string `mapstructure:"tls_key_file"`

	// ─────────────────────────────────────────────────────────────────────

	// DBPath is the SQLite database file path.
	DBPath string `mapstructure:"db_path"`

	// AdminPassword protects the admin dashboard with HTTP Basic Auth.
	// Leave empty to disable auth (not recommended in production).
	AdminPassword string `mapstructure:"admin_password"`

	// ReservedSubdomains is the list of subdomain names that clients cannot register.
	// Defaults to ["www", "api", "admin", "mail", "ftp"] when not specified.
	ReservedSubdomains []string `mapstructure:"reserved_subdomains"`

	// MaxTunnelsPerToken caps concurrent tunnels per auth token. 0 = unlimited.
	MaxTunnelsPerToken int `mapstructure:"max_tunnels_per_token"`

	// TCPMinPort and TCPMaxPort define the inclusive port range from which
	// the server allocates public TCP ports for `tunnd tcp <port>` tunnels.
	// Defaults: 20000–20100. Must be opened in the firewall and reachable
	// from the public internet for clients to use TCP tunneling.
	TCPMinPort int `mapstructure:"tcp_min_port"`
	TCPMaxPort int `mapstructure:"tcp_max_port"`

	// LogLevel: debug | info | warn | error
	LogLevel string `mapstructure:"log_level"`

	// LogFormat: pretty (human-readable) | json (structured, for log aggregators)
	LogFormat string `mapstructure:"log_format"`
}

// Client holds all client-side configuration.
type Client struct {
	// ServerAddr is the WebSocket URL of your Tunnd server.
	// Use wss:// for TLS (production), ws:// for plain HTTP (dev/local).
	ServerAddr string `mapstructure:"server_addr"`

	// Token is the auth token issued by the server admin.
	Token string `mapstructure:"token"`

	// Subdomain pins a specific subdomain. Random if empty.
	Subdomain string `mapstructure:"subdomain"`

	// Protocol: "http" (default) | "tcp"
	Protocol string `mapstructure:"protocol"`

	// InspectorPort is the local web inspector UI port. Default 4040.
	InspectorPort int `mapstructure:"inspector_port"`

	// LogLevel: debug | info | warn | error
	LogLevel string `mapstructure:"log_level"`
}

// LoadServerForCLI loads only the minimal config needed for CLI subcommands
// (token create/list/revoke). It skips full validation so domain, TLS, etc.
// are not required — only DBPath is needed.
func LoadServerForCLI(cfgFile string) (*Server, error) {
	v := viper.New()
	v.SetDefault("db_path", "./tunnd.db")
	v.SetDefault("admin_port", 9091)
	v.SetDefault("http_port", 443)
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "pretty")

	for key, env := range map[string]string{
		"db_path":    "TUNND_DB_PATH",
		"admin_port": "TUNND_ADMIN_PORT",
		"http_port":  "TUNND_HTTP_PORT",
		"log_level":  "TUNND_LOG_LEVEL",
		"log_format": "TUNND_LOG_FORMAT",
	} {
		v.BindEnv(key, env) //nolint:errcheck
	}

	if err := loadConfigFile(v, cfgFile, "tunnd-server"); err != nil {
		return nil, err
	}
	var cfg Server
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// LoadServer reads server config from env vars + optional config file.
// Configuration search order (highest to lowest precedence):
//  1. Path specified by cfgFile parameter (--config flag)
//  2. ./tunnd-server.yaml (current working directory)
//  3. ~/.tunnd/tunnd-server.yaml (user home directory)
//  4. /etc/tunnd/tunnd-server.yaml (system-wide)
func LoadServer(cfgFile string) (*Server, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("http_port", 443)
	v.SetDefault("admin_port", 9091)
	v.SetDefault("db_path", "./tunnd.db")
	v.SetDefault("acme_cache_dir", "./.autocert-cache")
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "pretty")
	v.SetDefault("max_tunnels_per_token", 0)
	v.SetDefault("tcp_min_port", 20000)
	v.SetDefault("tcp_max_port", 20100)

	// Explicit env bindings — avoids viper's ambiguous key-replacer behaviour.
	for key, env := range map[string]string{
		"domain":                "TUNND_DOMAIN",
		"http_port":             "TUNND_HTTP_PORT",
		"admin_port":            "TUNND_ADMIN_PORT",
		"tls_email":             "TUNND_TLS_EMAIL",
		"acme_cache_dir":        "TUNND_ACME_CACHE_DIR",
		"tls_cert_file":         "TUNND_TLS_CERT_FILE",
		"tls_key_file":          "TUNND_TLS_KEY_FILE",
		"db_path":               "TUNND_DB_PATH",
		"admin_password":        "TUNND_ADMIN_PASSWORD",
		"reserved_subdomains":   "TUNND_RESERVED_SUBDOMAINS",
		"max_tunnels_per_token": "TUNND_MAX_TUNNELS_PER_TOKEN",
		"tcp_min_port":          "TUNND_TCP_MIN_PORT",
		"tcp_max_port":          "TUNND_TCP_MAX_PORT",
		"log_level":             "TUNND_LOG_LEVEL",
		"log_format":            "TUNND_LOG_FORMAT",
	} {
		v.BindEnv(key, env) //nolint:errcheck
	}

	if err := loadConfigFile(v, cfgFile, "tunnd-server"); err != nil {
		return nil, err
	}

	var cfg Server
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, validateServer(&cfg)
}

// LoadClient reads client config from env vars + optional config file + CLI overrides.
// Configuration search order (highest to lowest precedence):
//  1. Path specified by cfgFile parameter (--config flag)
//  2. ./tunnd.yaml (current working directory)
//  3. ~/.tunnd/tunnd.yaml (user home directory)
//  4. /etc/tunnd/tunnd.yaml (system-wide)
func LoadClient(cfgFile string, cliOverrides map[string]interface{}) (*Client, error) {
	v := viper.New()

	v.SetDefault("protocol", "http")
	v.SetDefault("inspector_port", 4040)
	v.SetDefault("log_level", "info")

	for key, env := range map[string]string{
		"server_addr":    "TUNND_SERVER_ADDR",
		"token":          "TUNND_TOKEN",
		"subdomain":      "TUNND_SUBDOMAIN",
		"protocol":       "TUNND_PROTOCOL",
		"inspector_port": "TUNND_INSPECTOR_PORT",
		"log_level":      "TUNND_LOG_LEVEL",
	} {
		v.BindEnv(key, env) //nolint:errcheck
	}

	if err := loadConfigFile(v, cfgFile, "tunnd"); err != nil {
		return nil, err
	}

	// Apply CLI overrides (highest precedence)
	for key, value := range cliOverrides {
		v.Set(key, value)
	}

	var cfg Client
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, cfg.Validate()
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// loadConfigFile implements explicit file search order:
//  1. cfgFile (if provided via --config flag)
//  2. ./[defaultName].yaml (current working directory)
//  3. ~/.tunnd/[defaultName].yaml (user home directory)
//  4. /etc/tunnd/[defaultName].yaml (system-wide)
//
// A missing config file is not fatal — env vars and built-in defaults can
// fully describe a working configuration. We only fail when a file is
// found but unparseable.
func loadConfigFile(v *viper.Viper, cfgFile, defaultName string) error {
	// If explicit config file specified, use it when present, otherwise fall
	// through to env-driven configuration.
	if cfgFile != "" {
		if _, err := os.Stat(cfgFile); err != nil {
			if os.IsNotExist(err) {
				return nil // env vars / defaults will provide values
			}
			return fmt.Errorf("stat config file %s: %w", cfgFile, err)
		}
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file %s: %w", cfgFile, err)
		}
		return nil
	}

	// Search paths in order of precedence
	searchPaths := []string{
		"./" + defaultName + ".yaml", // Current working directory
	}

	// Add user home directory path
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, home+"/.tunnd/"+defaultName+".yaml")
	}

	// Add system-wide path
	searchPaths = append(searchPaths, "/etc/tunnd/"+defaultName+".yaml")

	// Try each path in order until we find one that exists
	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			// File exists, try to read it
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				// File found but unparseable — return error with file path
				return fmt.Errorf("failed to parse config file %s: %w", path, err)
			}
			return nil
		}
	}

	// No config file found in any search path — this is OK, we'll use defaults + env vars
	return nil
}

// Validate checks all Server configuration fields for correctness.
// It validates required fields and applies defaults for invalid optional fields.
// Returns a descriptive error for any required field that is missing or invalid.
func (s *Server) Validate() error {
	if s.Domain == "" {
		return fmt.Errorf(
			"'domain' is required.\n" +
				"  Set env:  export TUNND_DOMAIN=tunnel.example.com\n" +
				"  Or file:  domain: tunnel.example.com",
		)
	}
	// Strip accidental trailing punctuation from domain
	s.Domain = strings.TrimRight(s.Domain, "./:")

	// Require TLS only on port 443 (production). Any other port = dev/local mode.
	if s.HTTPPort == 443 && s.TLSEmail == "" && s.TLSCertFile == "" {
		return fmt.Errorf(
			"TLS is required on port 443. Choose one:\n" +
				"  Auto (Let's Encrypt): export TUNND_TLS_EMAIL=you@example.com\n" +
				"  Manual cert:          export TUNND_TLS_CERT_FILE=/path/to/cert.pem\n" +
				"                        export TUNND_TLS_KEY_FILE=/path/to/key.pem\n" +
				"  Dev/local (no TLS):   export TUNND_HTTP_PORT=8080",
		)
	}

	// admin_password is optional at config level since the bootstrap flow
	// sets it via the dashboard on first run. The server startup code is
	// responsible for warning about weak / default values — Validate doesn't
	// block startup on them.

	// Validate optional field: log_level — apply default if invalid.
	switch s.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		s.LogLevel = "info"
	}

	// Validate optional field: log_format — apply default if invalid.
	switch s.LogFormat {
	case "pretty", "json":
		// valid
	default:
		s.LogFormat = "pretty"
	}

	// Apply / clamp TCP port range defaults.
	if s.TCPMinPort == 0 {
		s.TCPMinPort = 20000
	}
	if s.TCPMaxPort == 0 {
		s.TCPMaxPort = 20100
	}
	if s.TCPMinPort < 1024 || s.TCPMinPort > 65535 ||
		s.TCPMaxPort < 1024 || s.TCPMaxPort > 65535 ||
		s.TCPMinPort > s.TCPMaxPort {
		return fmt.Errorf(
			"invalid TCP port range [%d, %d]: both must be in 1024–65535 and min ≤ max",
			s.TCPMinPort, s.TCPMaxPort,
		)
	}

	return nil
}

// Validate checks all Client configuration fields for correctness.
// It validates required fields and applies defaults for invalid optional fields.
// Returns a descriptive error for any required field that is missing or invalid.
func (c *Client) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf(
			"'server_addr' is required.\n" +
				"  Set env:  export TUNND_SERVER_ADDR=wss://tunnel.example.com\n" +
				"  Or file:  server_addr: wss://tunnel.example.com",
		)
	}

	if c.Token == "" {
		return fmt.Errorf(
			"'token' is required.\n" +
				"  Set env:  export TUNND_TOKEN=<your-token>\n" +
				"  Or file:  token: <your-token>",
		)
	}

	// Validate optional field: protocol — apply default if invalid.
	switch c.Protocol {
	case "http", "tcp":
		// valid
	default:
		c.Protocol = "http"
	}

	// Validate optional field: inspector_port — apply default if out of range.
	if c.InspectorPort < 1024 || c.InspectorPort > 65535 {
		c.InspectorPort = 4040
	}

	// Validate optional field: log_level — apply default if invalid.
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		c.LogLevel = "info"
	}

	return nil
}

func validateServer(cfg *Server) error {
	return cfg.Validate()
}
