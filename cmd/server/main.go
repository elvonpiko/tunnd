// Command tunnd-server is the Tunnd tunnel server.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"

	"github.com/elvonpiko/tunnd/internal/admin"
	"github.com/elvonpiko/tunnd/internal/auth"
	"github.com/elvonpiko/tunnd/internal/config"
	"github.com/elvonpiko/tunnd/internal/control"
	"github.com/elvonpiko/tunnd/internal/store"
	"github.com/elvonpiko/tunnd/internal/tunnel"
)

// Injected at build time via -ldflags.
var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

var cfgFile string

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tunnd-server",
		Short: "Tunnd tunnel server",
		Long: `Tunnd server — expose local services to the internet.

Run on a VPS with a wildcard DNS A record pointing to it:
  *.tunnel.yourdomain.com → <server-ip>

Clients connect with: tunnd http <port>`,
		RunE: runServer,
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	root.AddCommand(tokenCmd(), versionCmd())
	return root
}

// ── Server ────────────────────────────────────────────────────────────────────

func runServer(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadServer(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	setupLogging(cfg.LogLevel, cfg.LogFormat)

	// Warn if a legacy admin_password is set via config/env — it still works
	// but the preferred flow is the dashboard bootstrap on first run.
	if cfg.AdminPassword != "" {
		weakDefaults := map[string]bool{"changeme": true, "admin": true, "changeme-please": true}
		if weakDefaults[cfg.AdminPassword] {
			log.Warn().Msg("Admin password is set to a default value — change it immediately.")
		} else if len(cfg.AdminPassword) < 12 {
			log.Warn().Msg("Admin password is shorter than the recommended 12 characters.")
		}
	}

	log.Info().
		Str("domain", cfg.Domain).
		Int("http_port", cfg.HTTPPort).
		Int("admin_port", cfg.AdminPort).
		Str("version", Version).
		Msg("starting tunnd server")

	// ── 10.2: Create required directories ────────────────────────────────────
	if err := ensureDir(filepath.Dir(cfg.DBPath)); err != nil {
		return fmt.Errorf("creating DB directory %s: %w", filepath.Dir(cfg.DBPath), err)
	}
	// Only ensure the ACME cache dir when autocert is actually going to use it.
	// Manual-cert and no-TLS deployments don't need it, and creating it under
	// a read-only working directory (e.g. inside a Docker image's /) just
	// surfaces a confusing permission error at startup.
	if cfg.TLSEmail != "" && cfg.ACMECacheDir != "" {
		if err := ensureDir(cfg.ACMECacheDir); err != nil {
			return fmt.Errorf("creating ACME cache directory %s: %w", cfg.ACMECacheDir, err)
		}
	}

	// ── 10.3: Probe port availability ─────────────────────────────────────────
	if err := probePort(cfg.HTTPPort); err != nil {
		return fmt.Errorf("cannot bind to HTTP port %d: %w", cfg.HTTPPort, err)
	}
	if cfg.HTTPPort != cfg.AdminPort {
		if err := probePort(cfg.AdminPort); err != nil {
			return fmt.Errorf("cannot bind to admin port %d: %w", cfg.AdminPort, err)
		}
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()
	log.Info().Str("path", cfg.DBPath).Msg("database opened")

	authSvc := auth.New(db)
	reserved := cfg.ReservedSubdomains
	if len(reserved) == 0 {
		reserved = nil // use default reserved list inside NewWithValidator
	}
	registry := tunnel.NewWithValidator(db, cfg.Domain, reserved)
	registry.SetTCPPortRange(cfg.TCPMinPort, cfg.TCPMaxPort)

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}

	adminHandler := admin.New(authSvc, registry, db, cfg.AdminPassword)

	// ── Public mux: tunnel traffic + WebSocket control plane + admin ─────────
	// Requests to the bare base domain are routed to the admin dashboard so
	// operators can reach it over HTTPS at https://<domain> without exposing
	// the plain-HTTP admin port. Subdomain traffic goes to the tunnel registry.
	publicMux := http.NewServeMux()
	publicMux.Handle("/_tunnd/control", control.New(authSvc, registry, cfg.Domain))
	publicMux.Handle("/", rootHandler(cfg.Domain, registry, adminHandler))

	publicSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      publicMux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── Admin server ──────────────────────────────────────────────────────────
	// The same handler is also exposed on the dedicated admin port for
	// reverse-proxy and local-network access.
	adminSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.AdminPort),
		Handler:      adminHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)

	go func() {
		log.Info().Str("addr", publicSrv.Addr).Msg("public listener starting")
		if tlsConfig != nil {
			errCh <- publicSrv.ListenAndServeTLS("", "")
		} else {
			errCh <- publicSrv.ListenAndServe()
		}
	}()

	go func() {
		log.Info().Str("addr", adminSrv.Addr).Msg("admin listener starting")
		errCh <- adminSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	log.Info().Msg("shutting down gracefully…")
	publicSrv.Shutdown(shutCtx) //nolint:errcheck
	adminSrv.Shutdown(shutCtx)  //nolint:errcheck
	log.Info().Msg("server stopped")
	return nil
}

// ── TLS ───────────────────────────────────────────────────────────────────────

// buildTLSConfig returns a *tls.Config based on the server configuration.
//
// Priority:
//  1. Manual cert files (tls_cert_file + tls_key_file) — bring your own cert
//  2. Let's Encrypt via ACME autocert (tls_email set)
//  3. No TLS — plain HTTP (dev/local mode, only allowed when http_port != 443)
//
// Note on wildcards: Let's Encrypt HTTP-01 challenge cannot issue wildcard certs.
// For wildcard support you need DNS-01 — use manual certs (e.g. from Certbot
// with the --dns-* plugin) or set tls_cert_file/tls_key_file. The autocert path
// here issues per-subdomain certs on demand, which works fine for tunnels but
// requires port 80 to be accessible for the HTTP-01 challenge.
func buildTLSConfig(cfg *config.Server) (*tls.Config, error) {
	// ── 1. Manual certs (highest priority) ───────────────────────────────────
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading cert pair (%s, %s): %w",
				cfg.TLSCertFile, cfg.TLSKeyFile, err)
		}
		log.Info().
			Str("cert", cfg.TLSCertFile).
			Str("key", cfg.TLSKeyFile).
			Msg("TLS: using manual certificate")
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			CipherSuites: secureCiphers(),
			NextProtos:   []string{"h2", "http/1.1"},
		}, nil
	}

	// ── 2. Let's Encrypt autocert ─────────────────────────────────────────────
	if cfg.TLSEmail != "" {
		cacheDir := cfg.ACMECacheDir
		if cacheDir == "" {
			cacheDir = "./.autocert-cache"
		}
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			return nil, fmt.Errorf("creating ACME cache dir %s: %w", cacheDir, err)
		}

		m := &autocert.Manager{
			Prompt: autocert.AcceptTOS,
			// HostPolicy allows the base domain and any single-level subdomain.
			// autocert issues individual certs per subdomain on first request
			// (HTTP-01 challenge). This means the first request to a new subdomain
			// may be slow (~2s) while the cert is issued; subsequent requests use
			// the cached cert.
			HostPolicy: autocert.HostWhitelist(), // replaced at runtime — see below
			Cache:      autocert.DirCache(cacheDir),
			Email:      cfg.TLSEmail,
		}

		// Allow any subdomain of our base domain dynamically.
		m.HostPolicy = func(ctx context.Context, host string) error {
			// Allow exact domain and any *.domain subdomain
			if host == cfg.Domain {
				return nil
			}
			suffix := "." + cfg.Domain
			if len(host) > len(suffix) && host[len(host)-len(suffix):] == suffix {
				return nil
			}
			return fmt.Errorf("host %q not allowed", host)
		}

		// Start HTTP-01 challenge responder on port 80.
		// This must be reachable from the internet for Let's Encrypt to work.
		go func() {
			log.Info().Msg("ACME: HTTP-01 challenge server listening on :80")
			srv := &http.Server{
				Addr:         ":80",
				Handler:      m.HTTPHandler(nil),
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			}
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("ACME challenge server error")
			}
		}()

		log.Info().
			Str("email", cfg.TLSEmail).
			Str("cache", cacheDir).
			Str("domain", cfg.Domain).
			Msg("TLS: Let's Encrypt autocert enabled")

		return &tls.Config{
			GetCertificate: m.GetCertificate,
			MinVersion:     tls.VersionTLS12,
			CipherSuites:   secureCiphers(),
			NextProtos:     []string{"h2", "http/1.1"},
		}, nil
	}

	// ── 3. No TLS — dev/local mode ────────────────────────────────────────────
	log.Warn().
		Int("http_port", cfg.HTTPPort).
		Msg("TLS: no cert configured — running plain HTTP (dev mode)")
	log.Warn().Msg("TLS: do NOT run without TLS in production")
	return nil, nil
}

// secureCiphers returns a modern cipher suite list (TLS 1.2+).
func secureCiphers() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	}
}

// ── Token CLI ─────────────────────────────────────────────────────────────────

func tokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage auth tokens",
	}

	create := &cobra.Command{
		Use:   "create [label]",
		Short: "Create a new auth token",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			label := "default"
			if len(args) > 0 {
				label = args[0]
			}
			maxTunnels, _ := cmd.Flags().GetInt("max-tunnels")
			tok, err := auth.New(db).CreateToken(label, maxTunnels)
			if err != nil {
				return err
			}
			fmt.Printf("\nToken created!\n")
			fmt.Printf("  Label: %s\n", tok.Label)
			fmt.Printf("  Value: %s\n\n", tok.Value)
			fmt.Println("Store this value securely — it won't be shown again.")
			fmt.Printf("\nClient usage:\n  export TUNND_TOKEN=%s\n", tok.Value)
			return nil
		},
	}
	create.Flags().Int("max-tunnels", 0, "maximum concurrent tunnels (0 = unlimited)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List all auth tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tokens, err := auth.New(db).ListTokens()
			if err != nil {
				return err
			}
			if len(tokens) == 0 {
				fmt.Println("No tokens. Create one with: tunnd-server token create <label>")
				return nil
			}
			fmt.Printf("\n%-36s  %-20s  %-20s  %s\n", "ID", "Label", "Value (hint)", "Status")
			fmt.Println(repeat("-", 90))
			for _, t := range tokens {
				hint := t.Value
				if len(hint) > 20 {
					hint = hint[:20] + "…"
				}
				status := "active"
				if !t.Enabled {
					status = "revoked"
				}
				fmt.Printf("%-36s  %-20s  %-20s  %s\n", t.ID, t.Label, hint, status)
			}
			fmt.Println()
			return nil
		},
	}

	revoke := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a token by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := auth.New(db).RevokeToken(args[0]); err != nil {
				return err
			}
			fmt.Printf("Token %s revoked.\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(create, list, revoke)
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tunnd-server %s (%s) built %s\n", Version, CommitSHA, BuildDate)
		},
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// rootHandler dispatches public requests by Host. Requests addressed to the
// bare base domain (e.g. https://tunnd.example.com) are served by the admin
// dashboard; everything else (subdomain tunnel traffic) goes to the registry.
// This lets operators reach the dashboard over HTTPS on the public domain
// while the admin port stays available for reverse-proxy / LAN access.
func rootHandler(domain string, registry *tunnel.Registry, adminHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i != -1 {
			host = host[:i]
		}
		if host == domain {
			adminHandler.ServeHTTP(w, r)
			return
		}
		registry.ServeHTTP(w, r)
	})
}

func openDB() (*store.DB, error) {
	// For token CLI commands we only need the DB path — skip full server validation
	// (which requires domain, TLS config, etc.) by reading the raw viper value.
	cfg, err := config.LoadServerForCLI(cfgFile)
	if err != nil {
		return nil, err
	}
	return store.Open(cfg.DBPath)
}

func setupLogging(level, format string) {
	if format == "pretty" || format == "" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
}

func repeat(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}

// ensureDir creates dir and all parent directories with the given permissions.
// Returns a descriptive error if creation fails (e.g., permission denied, disk full).
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// probePort attempts to open and immediately close a TCP listener on the given
// port. If another process is already bound to the port, the error will
// describe the conflict clearly (e.g. "address already in use").
func probePort(port int) error {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}
