// Command tunnd is the Tunnd tunnel client.
// Run `tunnd setup` on first use to configure your server and token.
// Then use `tunnd http <port>` or `tunnd tcp <port>` to open tunnels.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/elvonpiko/tunnd/internal/tunnel"
	"github.com/elvonpiko/tunnd/pkg/proto"
)

var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

// clientConfig holds all client runtime settings.
// Stored as JSON at ~/.config/tunnd/config.json.
type clientConfig struct {
	ServerAddr    string `json:"server_addr"`
	Token         string `json:"token"`
	InspectorPort int    `json:"inspector_port"`
	LogLevel      string `json:"log_level"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tunnd", "config.json")
}

func loadConfig() (*clientConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"tunnd is not set up yet.\n" +
					"  Run: tunnd setup",
			)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg clientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.InspectorPort == 0 {
		cfg.InspectorPort = 4040
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	return &cfg, nil
}

func saveConfig(cfg *clientConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "tunnd",
		Short: "Expose localhost to the internet",
		Long: `Tunnd — open a secure tunnel from your machine to the world.

First time? Run:
  tunnd setup

Then expose a local service:
  tunnd http 3000
  tunnd http 8080 --subdomain myapp
  tunnd tcp 5432`,
	}
	root.AddCommand(setupCmd(), httpCmd(), tcpCmd(), statusCmd(), updateCmd(), versionCmd())
	return root
}

// ── Setup wizard ──────────────────────────────────────────────────────────────

func setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure Tunnd (server address and auth token)",
		Long: `Interactive setup wizard.

Run this once after installing to connect Tunnd to your server.
Your settings are saved to ~/.config/tunnd/config.json.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup()
		},
	}
}

func runSetup() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println()
	fmt.Println("  ▲  Tunnd Setup")
	fmt.Println()

	// Check for existing config
	existing, err := loadConfig()
	if err == nil && existing.ServerAddr != "" {
		fmt.Printf("  Current server: %s\n", existing.ServerAddr)
		fmt.Print("  Reconfigure? [y/N] ")
		scanner.Scan()
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			fmt.Println("  No changes made.")
			return nil
		}
		fmt.Println()
	}

	// Step 1: server address
	fmt.Print("  Server address (e.g. wss://tunnd.example.com): ")
	scanner.Scan()
	serverAddr := strings.TrimSpace(scanner.Text())
	if serverAddr == "" {
		return fmt.Errorf("server address is required")
	}
	if !strings.HasPrefix(serverAddr, "ws://") && !strings.HasPrefix(serverAddr, "wss://") {
		return fmt.Errorf("server address must start with wss:// (or ws:// for local dev)")
	}

	// Verify the server is reachable
	fmt.Print("  Checking server… ")
	if err := checkServer(serverAddr); err != nil {
		fmt.Println("✗")
		fmt.Println()
		fmt.Printf("  Could not reach %s\n", serverAddr)
		fmt.Printf("  Error: %s\n", err.Error())
		fmt.Println()
		fmt.Println("  Make sure the server is running and the address is correct.")
		fmt.Println("  Use ws:// instead of wss:// for local dev without TLS.")
		return fmt.Errorf("server unreachable")
	}
	fmt.Println("✓")

	// Step 2: token
	fmt.Println()
	fmt.Println("  Create a token in your admin dashboard (Tokens tab → + New Token),")
	fmt.Println("  then paste it here.")
	fmt.Println()
	fmt.Print("  Auth token (tnnd_...): ")
	scanner.Scan()
	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return fmt.Errorf("auth token is required")
	}
	if !strings.HasPrefix(token, "tnnd_") {
		fmt.Println()
		fmt.Println("  ⚠  Token doesn't look right (expected tnnd_... prefix).")
		fmt.Print("  Continue anyway? [y/N] ")
		scanner.Scan()
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			return fmt.Errorf("setup cancelled")
		}
	}

	cfg := &clientConfig{
		ServerAddr:    serverAddr,
		Token:         token,
		InspectorPort: 4040,
		LogLevel:      "info",
	}
	if err := saveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Println("  ✓ All set!")
	fmt.Println()
	fmt.Println("  Open a tunnel:")
	fmt.Println("    tunnd http 3000")
	fmt.Println("    tunnd http 8080 --subdomain myapp")
	fmt.Println()
	return nil
}

// checkServer does a lightweight HTTP GET to the admin stats endpoint
// to verify the server is up before asking for a token. Returns a
// detailed error so users can tell DNS / TLS / timeout / unexpected
// status codes apart at a glance.
func checkServer(wsAddr string) error {
	// Convert wss:// → https://, ws:// → http://
	httpAddr := strings.Replace(wsAddr, "wss://", "https://", 1)
	httpAddr = strings.Replace(httpAddr, "ws://", "http://", 1)
	httpAddr = strings.TrimRight(httpAddr, "/") + "/api/stats"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(httpAddr) //nolint:noctx
	if err != nil {
		// Surface the specific transport failure (DNS, TLS, connect, timeout)
		// instead of a generic "could not be reached".
		return fmt.Errorf("%s: %w", httpAddr, err)
	}
	defer resp.Body.Close()
	// 200 = no auth needed (bootstrap), 401 = server up but auth required,
	// 303 = unauthenticated UI redirect to /login, 503 = bootstrap in progress —
	// all mean the server is reachable and responding sensibly.
	switch resp.StatusCode {
	case http.StatusOK,
		http.StatusUnauthorized,
		http.StatusSeeOther,
		http.StatusServiceUnavailable:
		return nil
	}
	return fmt.Errorf("%s: server responded with HTTP %d", httpAddr, resp.StatusCode)
}

// ── http / tcp commands ───────────────────────────────────────────────────────

func httpCmd() *cobra.Command {
	var (
		subdomain          string
		inspectorPort      int
		inspChanged        bool
		upstreamScheme     string
		upstreamSkipVerify bool
		hostHeader         string
	)
	cmd := &cobra.Command{
		Use:   "http <port>",
		Short: "Tunnel an HTTP service on localhost",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			// Validate --upstream-scheme early so a bad value fails fast with a
			// clear message instead of surfacing later as a confusing dial error.
			// Empty string is treated as "http" (the default) and is accepted.
			switch upstreamScheme {
			case "", "http", "https":
			default:
				return fmt.Errorf("invalid --upstream-scheme %q: must be \"http\" or \"https\"", upstreamScheme)
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if subdomain != "" {
				validator := tunnel.NewSubdomainValidator(nil)
				if _, err := validator.ValidateAndSanitize(subdomain); err != nil {
					return fmt.Errorf("invalid subdomain %q: %s", subdomain, err.Error())
				}
			}
			if inspChanged {
				cfg.InspectorPort = inspectorPort
			}
			// Auto-detect HTTPS upstream when the user didn't pin --upstream-scheme.
			// This makes `tunnd http <port>` "just work" against vite/next dev
			// servers regardless of whether they're plain HTTP or self-signed
			// HTTPS. Skip-verify is auto-set unless the user opted in/out.
			if !cmd.Flags().Changed("upstream-scheme") {
				detected, detSkipVerify := probeUpstreamScheme(port)
				upstreamScheme = detected
				if detected == "https" {
					log.Info().Int("port", port).Msg("detected HTTPS upstream")
				}
				if !cmd.Flags().Changed("upstream-tls-skip-verify") {
					upstreamSkipVerify = detSkipVerify
				}
			}
			return runTunnel(cfg, "http", subdomain, port, upstreamScheme, upstreamSkipVerify, hostHeader)
		},
	}
	cmd.Flags().StringVarP(&subdomain, "subdomain", "s", "", "pin a subdomain (random if not set)")
	cmd.Flags().IntVar(&inspectorPort, "inspector-port", 4040, "local inspector UI port (0 to disable)")
	cmd.Flags().StringVar(&upstreamScheme, "upstream-scheme", "http", `upstream protocol: "http" or "https" (default "http")`)
	cmd.Flags().BoolVar(&upstreamSkipVerify, "upstream-tls-skip-verify", false, "skip TLS verification on the upstream (use with self-signed dev certs)")
	cmd.Flags().StringVar(&hostHeader, "host-header", "rewrite", `host header policy: "rewrite" (default — replace with localhost:<port>), "preserve" (forward public Host), or a literal hostname`)
	// Track if inspector-port was explicitly set
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		inspChanged = cmd.Flags().Changed("inspector-port")
		return nil
	}
	return cmd
}

func tcpCmd() *cobra.Command {
	var subdomain string
	return &cobra.Command{
		Use:   "tcp <port>",
		Short: "Tunnel a raw TCP port on localhost",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := parsePort(args[0])
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			cfg.InspectorPort = 0 // TCP never uses inspector
			// Raw TCP has no HTTP layer, so upstream-scheme / skip-verify
			// don't apply. The flags are not registered on tcpCmd.
			return runTunnel(cfg, "tcp", subdomain, port, "", false, "")
		},
	}
}

// ── Status command ────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			fmt.Println()
			fmt.Println("  ▲  Tunnd")
			fmt.Println()
			fmt.Printf("  Server:  %s\n", cfg.ServerAddr)
			hint := cfg.Token
			if len(hint) > 20 {
				hint = hint[:20] + "…"
			}
			fmt.Printf("  Token:   %s\n", hint)
			fmt.Printf("  Config:  %s\n", configPath())
			fmt.Println()
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tunnd %s (%s) built %s\n", Version, CommitSHA, BuildDate)
		},
	}
}

// ── Inspector log ─────────────────────────────────────────────────────────────

type requestEntry struct {
	ID         string    `json:"id"`
	Method     string    `json:"method"`
	URL        string    `json:"url"`
	StatusCode int       `json:"status_code"`
	DurationMs int64     `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

var (
	inspMu  sync.Mutex
	inspLog []requestEntry
)

func logRequest(method, url string, status int, dur time.Duration) {
	inspMu.Lock()
	defer inspMu.Unlock()
	inspLog = append(inspLog, requestEntry{
		ID:         fmt.Sprintf("%d", len(inspLog)+1),
		Method:     method,
		URL:        url,
		StatusCode: status,
		DurationMs: dur.Milliseconds(),
		Timestamp:  time.Now(),
	})
	if len(inspLog) > 500 {
		inspLog = inspLog[len(inspLog)-500:]
	}
}

// ── Tunnel client ─────────────────────────────────────────────────────────────

// reqBuffer is a thread-safe, non-blocking-on-write byte buffer used to hold
// HTTP request bytes that arrive from the WebSocket before the local
// connection has been dialed. Writers append (non-blocking); readers block
// until bytes are available or the buffer is closed.
type reqBuffer struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func newReqBuffer() *reqBuffer {
	b := &reqBuffer{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func (b *reqBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	b.buf = append(b.buf, p...)
	b.cond.Broadcast()
	return len(p), nil
}

func (b *reqBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.buf) == 0 && !b.closed {
		b.cond.Wait()
	}
	if len(b.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b.buf)
	b.buf = b.buf[n:]
	return n, nil
}

// Close signals "no more writes" so any blocked reader returns io.EOF after
// draining remaining bytes.
func (b *reqBuffer) Close() error {
	b.mu.Lock()
	b.closed = true
	b.cond.Broadcast()
	b.mu.Unlock()
	return nil
}

// clientStream tracks one in-flight tunneled request on the client side.
//
// Lifecycle:
//  1. MsgOpen arrives → openStream creates a clientStream synchronously,
//     registers it in tc.streams, and spawns a goroutine to dial the local
//     service. Any MsgData / MsgReqDone frames arriving before the dial
//     completes are buffered (req buffer) / latched (reqDone channel).
//  2. The dial goroutine connects, drains the buffered request bytes into
//     the local socket, then waits for reqDone before half-closing the
//     write side and reading the response back through the WebSocket.
//  3. On error or completion, the local conn is closed, the stream removed,
//     and a MsgClose frame is sent to the server.
type clientStream struct {
	id string

	// req is a non-blocking-write buffer that the readLoop appends to as
	// MsgData frames arrive. The driveStream goroutine drains it into the
	// local conn once the dial has succeeded.
	req *reqBuffer

	// reqDone is closed when the server signals MsgReqDone — the dial
	// goroutine waits on this before half-closing the local conn's write side.
	reqDone chan struct{}

	// localConn is set once the dial succeeds. nil before that.
	localMu   sync.Mutex
	localConn net.Conn
	closed    bool
}

// closeOnce closes the request buffer and local conn at most once.
func (cs *clientStream) closeOnce() {
	cs.localMu.Lock()
	defer cs.localMu.Unlock()
	if cs.closed {
		return
	}
	cs.closed = true
	cs.req.Close() //nolint:errcheck
	if cs.localConn != nil {
		cs.localConn.Close() //nolint:errcheck
	}
}

// signalReqDone closes the reqDone channel exactly once.
func (cs *clientStream) signalReqDone() {
	select {
	case <-cs.reqDone:
		// already closed
	default:
		close(cs.reqDone)
	}
}

type tunnelClient struct {
	cfg       *clientConfig
	protocol  string
	subdomain string
	localPort int

	// upstreamScheme controls how the client dials the local upstream.
	// "" / "http" → plain TCP. "https" → TCP + TLS handshake (Phase 5).
	// Validated at flag-parse time in httpCmd; not registered for tcpCmd.
	upstreamScheme string

	// upstreamSkipVerify, when true, disables TLS-cert verification on the
	// upstream conn. Local-only — does NOT cross the wire.
	upstreamSkipVerify bool

	// hostHeader controls how the public Host header is forwarded to the
	// upstream. "" / "rewrite" → replace with localhost:<localPort> on the
	// server. "preserve" → forward verbatim. Any other value → literal
	// hostname applied verbatim. Sent to the server in RegisterPayload;
	// not registered on tcpCmd (raw TCP has no HTTP layer).
	hostHeader string

	connMu sync.Mutex
	conn   *websocket.Conn

	tunnelID  string
	publicURL string

	streamsMu sync.RWMutex
	streams   map[string]*clientStream
}

type fatalError struct{ cause error }

func (e *fatalError) Error() string { return e.cause.Error() }
func (e *fatalError) Unwrap() error { return e.cause }

func runTunnel(cfg *clientConfig, protocol, subdomain string, localPort int, upstreamScheme string, upstreamSkipVerify bool, hostHeader string) error {
	setupLogging(cfg.LogLevel)
	tc := &tunnelClient{
		cfg:                cfg,
		protocol:           protocol,
		subdomain:          subdomain,
		localPort:          localPort,
		upstreamScheme:     upstreamScheme,
		upstreamSkipVerify: upstreamSkipVerify,
		hostHeader:         hostHeader,
		streams:            make(map[string]*clientStream),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return tc.connectWithRetry(ctx)
}

const pongWait = 60 * time.Second

func (tc *tunnelClient) connectWithRetry(ctx context.Context) error {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if attempt > 0 {
			log.Info().Dur("retry_in", backoff).Int("attempt", attempt+1).Msg("reconnecting…")
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
		attempt++
		if err := tc.connect(ctx); err != nil {
			var fe *fatalError
			if errors.As(err, &fe) {
				return fe.cause
			}
			log.Error().Err(err).Msg("tunnel disconnected")
			continue
		}
		return nil
	}
}

func (tc *tunnelClient) connect(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	wsURL := tc.cfg.ServerAddr + "/_tunnd/control"

	log.Info().Str("server", tc.cfg.ServerAddr).Msg("connecting…")
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("cannot reach server: %w\n  Check: tunnd status", err)
	}
	defer conn.Close()

	tc.connMu.Lock()
	tc.conn = conn
	tc.connMu.Unlock()

	if err := tc.register(conn); err != nil {
		return fmt.Errorf("registration: %w", err)
	}

	if tc.protocol == "http" && tc.cfg.InspectorPort > 0 {
		go runInspector(tc.publicURL, tc.localPort, tc.cfg.InspectorPort)
	}

	return tc.readLoop(ctx, conn)
}

func (tc *tunnelClient) register(conn *websocket.Conn) error {
	msg, err := proto.EncodeJSON(proto.MsgRegister, proto.RegisterPayload{
		Token:          tc.cfg.Token,
		Subdomain:      tc.subdomain,
		Protocol:       tc.protocol,
		LocalPort:      tc.localPort,
		UpstreamScheme: tc.upstreamScheme,
		HostHeader:     tc.hostHeader,
	})
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		return fmt.Errorf("sending register: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, raw, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("reading server reply: %w", err)
	}
	if proto.FrameKind(raw) != proto.FrameKindJSON {
		return fmt.Errorf("expected JSON frame from server, got kind 0x%02x", proto.FrameKind(raw))
	}
	env, err := proto.DecodeJSON(raw)
	if err != nil {
		return err
	}
	switch env.Type {
	case proto.MsgRegistered:
		var reg proto.RegisteredPayload
		if err := proto.DecodePayload(env, &reg); err != nil {
			return err
		}
		tc.tunnelID = reg.TunnelID
		tc.publicURL = reg.PublicURL
		printBanner(reg.PublicURL, tc.localPort, tc.cfg.InspectorPort, tc.protocol)
		return nil
	case proto.MsgError:
		var ep proto.ErrorPayload
		proto.DecodePayload(env, &ep) //nolint:errcheck
		fmt.Fprintf(os.Stderr, "\n  ✗  %s\n", ep.Message)
		switch ep.Code {
		case "subdomain_in_use":
			fmt.Fprintf(os.Stderr, "     Try a different subdomain: tunnd http %d --subdomain myapp2\n", tc.localPort)
		case "handshake_failed":
			fmt.Fprintf(os.Stderr, "     Your token may be invalid or revoked.\n")
			fmt.Fprintf(os.Stderr, "     Run: tunnd setup   to reconfigure.\n")
		}
		fmt.Fprintln(os.Stderr)
		return &fatalError{cause: fmt.Errorf("registration rejected [%s]: %s", ep.Code, ep.Message)}
	default:
		return fmt.Errorf("unexpected frame during handshake: %s", env.Type)
	}
}

func (tc *tunnelClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go tc.pingLoop(ctx, conn)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("ws read: %w", err)
		}

		// Fast path: binary data frames carry stream payload bytes.
		if proto.FrameKind(raw) == proto.FrameKindBinaryData {
			streamID, payload, derr := proto.DecodeBinaryData(raw)
			if derr != nil {
				log.Warn().Err(derr).Msg("decoding binary data frame")
				continue
			}
			if cs := tc.getStream(streamID); cs != nil {
				if _, werr := cs.req.Write(payload); werr != nil {
					log.Debug().Err(werr).Str("stream", streamID).Msg("req buffer write")
				}
			} else {
				log.Debug().Str("stream", streamID).Msg("MsgData for unknown stream — dropped")
			}
			continue
		}

		env, err := proto.DecodeJSON(raw)
		if err != nil {
			log.Warn().Err(err).Msg("decoding frame")
			continue
		}
		switch env.Type {
		case proto.MsgOpen:
			var op proto.OpenPayload
			if err := proto.DecodePayload(env, &op); err != nil {
				continue
			}
			tc.openStream(conn, op.StreamID)

		case proto.MsgOpenTCP:
			var op proto.OpenPayload
			if err := proto.DecodePayload(env, &op); err != nil {
				continue
			}
			tc.openTCPStream(conn, op.StreamID)

		case proto.MsgReqDone:
			var rd proto.ReqDonePayload
			if err := proto.DecodePayload(env, &rd); err != nil {
				continue
			}
			if cs := tc.getStream(rd.StreamID); cs != nil {
				cs.req.Close() //nolint:errcheck
				cs.signalReqDone()
			}

		case proto.MsgClose:
			var cp proto.ClosePayload
			if err := proto.DecodePayload(env, &cp); err != nil {
				continue
			}
			tc.closeStream(cp.StreamID)

		case proto.MsgError:
			var ep proto.ErrorPayload
			proto.DecodePayload(env, &ep) //nolint:errcheck
			log.Error().Str("code", ep.Code).Str("msg", ep.Message).Msg("server error")

		case proto.MsgPong:
			// handled by SetPongHandler
		}
	}
}

// openStream is called synchronously from the readLoop when a new MsgOpen
// arrives. It registers the stream BEFORE returning so that any MsgData /
// MsgReqDone frames that follow immediately can be routed correctly. The
// actual local-service dial happens in a separate goroutine.
func (tc *tunnelClient) openStream(conn *websocket.Conn, streamID string) {
	cs := &clientStream{
		id:      streamID,
		req:     newReqBuffer(),
		reqDone: make(chan struct{}),
	}
	tc.streamsMu.Lock()
	tc.streams[streamID] = cs
	tc.streamsMu.Unlock()

	// Process the rest asynchronously so the readLoop can keep handling frames.
	go tc.driveStream(conn, cs)
}

// driveStream dials the local service, pumps the buffered request bytes in,
// reads the response back, and forwards it to the server.
func (tc *tunnelClient) driveStream(conn *websocket.Conn, cs *clientStream) {
	start := time.Now()
	defer func() {
		tc.removeStream(cs.id)
		cs.closeOnce()
		tc.sendFrame(conn, proto.MsgClose, proto.ClosePayload{StreamID: cs.id}) //nolint:errcheck
	}()

	localConn, err := dialLocal(tc.localPort, 10*time.Second)
	if err != nil {
		if isConnectionRefused(err) {
			log.Error().Int("port", tc.localPort).Msgf(
				"cannot connect to local service: no service listening on port %d — is your dev server running?",
				tc.localPort,
			)
		} else {
			log.Error().Err(err).Int("port", tc.localPort).Msg("cannot connect to local service")
		}
		// Drain any buffered request so writers don't block.
		cs.req.Close() //nolint:errcheck
		return
	}

	// If the upstream is HTTPS, wrap the dialed conn in a TLS client and
	// complete the handshake before any request bytes flow. ServerName is
	// "localhost" because we dial localhost:<port>; for self-signed dev
	// certs the user opts in to InsecureSkipVerify via
	// --upstream-tls-skip-verify.
	if tc.upstreamScheme == "https" {
		tlsConn, err := wrapUpstreamTLS(localConn, tc.upstreamSkipVerify, 10*time.Second)
		if err != nil {
			localConn.Close() //nolint:errcheck
			log.Error().Err(err).Int("port", tc.localPort).Msgf(
				"TLS handshake failed: %v; if the upstream uses a self-signed cert, pass --upstream-tls-skip-verify",
				err,
			)
			cs.req.Close() //nolint:errcheck
			return
		}
		localConn = tlsConn
	}

	cs.localMu.Lock()
	if cs.closed {
		cs.localMu.Unlock()
		localConn.Close() //nolint:errcheck
		return
	}
	cs.localConn = localConn
	cs.localMu.Unlock()

	// ── Pump request bytes from the buffer → local conn ─────────────────────
	// io.Copy returns when the buffer is closed (server signaled MsgReqDone)
	// or on write error. We use a deadline-resetting wrapper so a stuck local
	// service eventually surfaces a write timeout.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = io.Copy(deadlineWriter{conn: localConn, timeout: 30 * time.Second}, cs.req)
	}()
	<-pumpDone

	// Half-close the write side so the upstream local server knows the
	// request body is fully delivered (most servers won't respond otherwise).
	if tcpConn, ok := localConn.(*net.TCPConn); ok {
		tcpConn.CloseWrite() //nolint:errcheck
	}

	// ── Read response bytes from local conn → MsgData frames to server ──────
	buf := make([]byte, 32*1024)
	var (
		statusCode int
		method     = "GET"
		urlPath    = "/"
	)
	// Clear any stale read deadline left by an earlier path, then set a
	// single whole-stream cap of 60 minutes. This is the documented
	// long-stream limit: SSE / long-poll / chunked responses survive as
	// long as bytes flow within this window, even when the per-byte
	// interval exceeds 120s. Liveness for shorter intervals is enforced
	// at the wire level (WebSocket ping/pong) and at the upstream level
	// (TCP keepalive). See design.md Property P7 and bugfix.md clause 1.9.
	_ = localConn.SetReadDeadline(time.Time{})
	_ = localConn.SetReadDeadline(time.Now().Add(60 * time.Minute))
	for {
		n, err := localConn.Read(buf)
		if n > 0 {
			if statusCode == 0 && n > 12 {
				fmt.Sscanf(string(buf[9:12]), "%d", &statusCode)
			}
			if sendErr := tc.sendData(conn, cs.id, buf[:n]); sendErr != nil {
				log.Debug().Err(sendErr).Str("stream", cs.id).Msg("sending response data")
				break
			}
		}
		if err != nil {
			break
		}
	}

	if statusCode == 0 {
		statusCode = 502
	}
	logRequest(method, tc.publicURL+urlPath, statusCode, time.Since(start))
}

// openTCPStream is called for `tunnd tcp <port>` tunnels. Unlike the HTTP
// flow, TCP streams are fully bidirectional with no request/response
// distinction — we just dial the local TCP service and pipe bytes both ways.
func (tc *tunnelClient) openTCPStream(conn *websocket.Conn, streamID string) {
	cs := &clientStream{
		id:      streamID,
		req:     newReqBuffer(),
		reqDone: make(chan struct{}), // unused for TCP, but kept for symmetry
	}
	tc.streamsMu.Lock()
	tc.streams[streamID] = cs
	tc.streamsMu.Unlock()

	go tc.driveTCPStream(conn, cs)
}

// driveTCPStream connects to the local TCP service and pipes bytes between
// the WebSocket buffer and the local conn in both directions.
func (tc *tunnelClient) driveTCPStream(conn *websocket.Conn, cs *clientStream) {
	defer func() {
		tc.removeStream(cs.id)
		cs.closeOnce()
		tc.sendFrame(conn, proto.MsgClose, proto.ClosePayload{StreamID: cs.id}) //nolint:errcheck
	}()

	localConn, err := dialLocal(tc.localPort, 10*time.Second)
	if err != nil {
		if isConnectionRefused(err) {
			log.Error().Int("port", tc.localPort).Msgf(
				"cannot connect to local service: no service listening on port %d — is your dev server running?",
				tc.localPort,
			)
		} else {
			log.Error().Err(err).Int("port", tc.localPort).Msg("cannot connect to local TCP service")
		}
		cs.req.Close() //nolint:errcheck
		return
	}

	cs.localMu.Lock()
	if cs.closed {
		cs.localMu.Unlock()
		localConn.Close() //nolint:errcheck
		return
	}
	cs.localConn = localConn
	cs.localMu.Unlock()

	// Inbound (server → local): drain the buffer continuously into local conn.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = io.Copy(localConn, cs.req)
	}()

	// Outbound (local → server): read local conn, send binary data frames.
	buf := make([]byte, 32*1024)
	for {
		n, err := localConn.Read(buf)
		if n > 0 {
			if sendErr := tc.sendData(conn, cs.id, buf[:n]); sendErr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	<-pumpDone
}

// dialLocal opens a TCP connection to the local service on the given
// port. It dials "localhost:<port>" with a single Dialer call: the OS
// resolver returns both IPv4 and IPv6 addresses, and Go's net.Dialer
// runs Happy Eyeballs (RFC 8305) to race the families and pick the
// winner. This works identically on Linux, macOS, and Windows, and
// transparently handles the common "Vite binds to ::1 only on
// Windows" case without a sequential fallback. The wall clock is
// bounded by the supplied timeout.
func dialLocal(port int, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	d := net.Dialer{Timeout: timeout, DualStack: true}
	return d.DialContext(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
}

// isConnectionRefused reports whether err indicates the upstream actively
// refused the connection on every loopback family. Recognizes:
//   - POSIX ECONNREFUSED (Linux, macOS)
//   - Windows WSAECONNREFUSED, which surfaces as a wrapped
//     *os.SyscallError whose .Error() contains "actively refused"
//
// We use a substring match for the Windows form so the helper compiles
// and lints on every platform without `//go:build` tags.
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return strings.Contains(err.Error(), "actively refused")
}

// wrapUpstreamTLS wraps localConn in a TLS client and completes the handshake
// under a timeout. Used by driveStream when --upstream-scheme=https.
// ServerName is "localhost" because we dial localhost:<port>; skipVerify is
// opt-in for self-signed dev certs via --upstream-tls-skip-verify.
func wrapUpstreamTLS(localConn net.Conn, skipVerify bool, timeout time.Duration) (net.Conn, error) {
	tlsConn := tls.Client(localConn, &tls.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: skipVerify, //nolint:gosec // opt-in via --upstream-tls-skip-verify
		MinVersion:         tls.VersionTLS12,
	})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	return tlsConn, nil
}

// probeUpstreamScheme detects whether the local upstream on the given port
// speaks HTTP or HTTPS. It dials localhost:<port> with a short timeout and
// attempts an opportunistic TLS handshake (with cert verification skipped, so
// self-signed dev certs work). If the handshake completes, the upstream is
// HTTPS and we cache skipVerify=true (typical for dev). If the dial or
// handshake fails, we default to HTTP — the actual stream dial will surface
// any real error later.
//
// The probe is best-effort and bounded: ~500 ms total wall-clock budget in
// the success path, ~1 s in the failure path. It runs once at startup, not
// per-request.
func probeUpstreamScheme(port int) (scheme string, skipVerify bool) {
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer dialCancel()
	d := net.Dialer{Timeout: 300 * time.Millisecond, DualStack: true}
	conn, err := d.DialContext(dialCtx, "tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return "http", false
	}
	defer conn.Close() //nolint:errcheck

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         "localhost",
		InsecureSkipVerify: true, //nolint:gosec // probe only — discarded immediately after handshake
		MinVersion:         tls.VersionTLS12,
	})
	hsCtx, hsCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer hsCancel()
	if err := tlsConn.HandshakeContext(hsCtx); err != nil {
		return "http", false
	}
	return "https", true
}

// deadlineWriter wraps a net.Conn and resets its write deadline before every Write.
type deadlineWriter struct {
	conn    net.Conn
	timeout time.Duration
}

func (dw deadlineWriter) Write(p []byte) (int, error) {
	dw.conn.SetWriteDeadline(time.Now().Add(dw.timeout)) //nolint:errcheck
	return dw.conn.Write(p)
}

func (tc *tunnelClient) getStream(id string) *clientStream {
	tc.streamsMu.RLock()
	cs := tc.streams[id]
	tc.streamsMu.RUnlock()
	return cs
}

func (tc *tunnelClient) removeStream(id string) {
	tc.streamsMu.Lock()
	delete(tc.streams, id)
	tc.streamsMu.Unlock()
}

func (tc *tunnelClient) closeStream(id string) {
	cs := tc.getStream(id)
	if cs == nil {
		return
	}
	cs.signalReqDone()
	cs.closeOnce()
}

func (tc *tunnelClient) sendFrame(conn *websocket.Conn, msgType proto.MsgType, payload any) error {
	msg, err := proto.EncodeJSON(msgType, payload)
	if err != nil {
		return err
	}
	tc.connMu.Lock()
	defer tc.connMu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

// sendData sends a stream's raw payload bytes as a binary data frame.
// Used on the hot path for HTTP responses and TCP traffic — avoids the
// JSON+base64 cost of sendFrame for high-volume payloads.
func (tc *tunnelClient) sendData(conn *websocket.Conn, streamID string, payload []byte) error {
	frame, err := proto.EncodeBinaryData(streamID, payload)
	if err != nil {
		return err
	}
	tc.connMu.Lock()
	defer tc.connMu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (tc *tunnelClient) pingLoop(ctx context.Context, conn *websocket.Conn) {
	tick := time.NewTicker(45 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			tc.connMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			tc.connMu.Unlock()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// ── Inspector ─────────────────────────────────────────────────────────────────

func runInspector(publicURL string, localPort, inspectorPort int) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		inspMu.Lock()
		snapshot := make([]requestEntry, len(inspLog))
		copy(snapshot, inspLog)
		inspMu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"requests": snapshot,
			"count":    len(snapshot),
		})
	})

	mux.HandleFunc("POST /api/requests/clear", func(w http.ResponseWriter, r *http.Request) {
		inspMu.Lock()
		inspLog = inspLog[:0]
		inspMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"cleared": true}) //nolint:errcheck
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cfgJSON := fmt.Sprintf(`{"publicURL":%q,"localPort":%d,"inspectorPort":%d}`,
			publicURL, localPort, inspectorPort)
		fmt.Fprintf(w, inspectorHTMLHead, cfgJSON)
		fmt.Fprint(w, inspectorHTMLBody)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", inspectorPort)
	srv := &http.Server{Addr: addr, Handler: mux, ReadTimeout: 10 * time.Second}
	log.Info().Str("url", "http://"+addr).Msg("inspector started")
	srv.ListenAndServe() //nolint:errcheck
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q — must be 1–65535", s)
	}
	return p, nil
}

func printBanner(publicURL string, localPort, inspectorPort int, protocol string) {
	fmt.Println()
	fmt.Println("  ▲  Tunnd")
	fmt.Println()
	if protocol == "tcp" {
		fmt.Printf("  Forwarding    %s → localhost:%d (TCP)\n", publicURL, localPort)
	} else {
		fmt.Printf("  Forwarding    %s → localhost:%d\n", publicURL, localPort)
	}
	if protocol == "http" && inspectorPort > 0 {
		fmt.Printf("  Inspector     http://localhost:%d\n", inspectorPort)
	}
	fmt.Println()
	maybePrintUpdateHint()
	fmt.Println("  Ctrl+C to close tunnel")
	fmt.Println()
}

func setupLogging(level string) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
}

const inspectorHTMLHead = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tunnd Inspector</title>
<script>window.__TUNND__ = %s;</script>
</head>`

const inspectorHTMLBody = `
<body>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#09090b;--bg2:#111113;--bg3:#18181b;--border:rgba(255,255,255,.07);--border2:rgba(255,255,255,.13);--text:#f4f4f5;--muted:#71717a;--muted2:#a1a1aa;--accent:#a78bfa;--green:#34d399;--red:#f87171;--amber:#fbbf24;--blue:#60a5fa;--mono:'Berkeley Mono','Fira Code',monospace}
body{font-family:var(--mono);background:var(--bg);color:var(--text);font-size:13px;min-height:100vh;-webkit-font-smoothing:antialiased}
nav{padding:0 24px;height:52px;border-bottom:1px solid var(--border);display:flex;align-items:center;gap:12px;background:rgba(9,9,11,.9);backdrop-filter:blur(8px);position:sticky;top:0;z-index:10}
.logo{display:flex;align-items:center;gap:8px;font-weight:700;font-size:14px;color:var(--text)}
.logo-mark{width:22px;height:22px;background:var(--accent);border-radius:5px;display:flex;align-items:center;justify-content:center}
.logo-mark svg{width:12px;height:12px}
.divider{width:1px;height:20px;background:var(--border2)}
.nav-url{color:var(--accent);font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:340px}
.nav-local{color:var(--muted);font-size:12px}
.dot{width:7px;height:7px;border-radius:50%;background:var(--green);margin-left:auto;box-shadow:0 0 6px var(--green);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}
.count{font-size:11px;color:var(--muted);background:var(--bg3);border:1px solid var(--border);border-radius:100px;padding:2px 10px}
main{padding:20px 24px}
.toolbar{display:flex;align-items:center;justify-content:space-between;margin-bottom:14px}
.toolbar-title{font-size:11px;color:var(--muted);letter-spacing:.07em;text-transform:uppercase}
.btn-clear{font-family:var(--mono);font-size:11px;color:var(--muted);background:none;border:1px solid var(--border);border-radius:5px;padding:3px 10px;cursor:pointer;transition:.15s}
.btn-clear:hover{color:var(--text);border-color:var(--border2)}
.empty{text-align:center;color:var(--muted);padding:60px 0;font-size:13px}
.empty .hint{font-size:12px;margin-top:8px;opacity:.6}
.tw{background:var(--bg2);border:1px solid var(--border);border-radius:10px;overflow:hidden}
table{width:100%;border-collapse:collapse}
th{text-align:left;color:var(--muted);font-size:10px;letter-spacing:.08em;text-transform:uppercase;padding:9px 14px;border-bottom:1px solid var(--border);font-weight:500;white-space:nowrap}
td{padding:10px 14px;border-bottom:1px solid var(--border);color:var(--muted2);vertical-align:middle;white-space:nowrap}
td.path{color:var(--text);font-weight:500;white-space:normal;word-break:break-all;max-width:380px}
tr:last-child td{border-bottom:none}
tr:hover td{background:rgba(255,255,255,.015)}
td.s-ok{color:var(--green)}td.s-err{color:var(--red)}td.s-redir{color:var(--amber)}td.s-info{color:var(--blue)}
.badge{display:inline-flex;align-items:center;font-size:10px;font-weight:700;padding:2px 7px;border-radius:4px}
.GET{background:rgba(96,165,250,.12);color:var(--blue)}.POST{background:rgba(52,211,153,.1);color:var(--green)}
.PUT,.PATCH{background:rgba(251,191,36,.1);color:var(--amber)}.DELETE{background:rgba(248,113,113,.1);color:var(--red)}
.HEAD,.OPTIONS{background:rgba(161,161,170,.08);color:var(--muted2)}
.ts{color:var(--muted);font-size:11px}.dur{font-size:12px}.dur.slow{color:var(--amber)}.dur.ok{color:var(--green)}
.new-row{animation:newrow .4s ease}@keyframes newrow{from{background:rgba(167,139,250,.08)}to{background:transparent}}
</style>
<nav>
  <div class="logo"><div class="logo-mark"><svg viewBox="0 0 12 12" fill="none"><path d="M2 9L6 3L10 9" stroke="#09090b" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></div>Tunnd Inspector</div>
  <div class="divider"></div>
  <span class="nav-url" id="nav-url"></span>
  <span class="nav-local" id="nav-local"></span>
  <span class="count" id="nav-count">0 requests</span>
  <div class="dot"></div>
</nav>
<main>
  <div class="toolbar"><span class="toolbar-title">Request Log</span><button class="btn-clear" onclick="clearLog()">Clear</button></div>
  <div id="app"><div class="empty"><div>Waiting for requests…</div><div class="hint">Make a request to your tunnel URL to see it here.</div></div></div>
</main>
<script>
const cfg=window.__TUNND__;
document.getElementById('nav-url').textContent=cfg.publicURL;
document.getElementById('nav-local').textContent='→ localhost:'+cfg.localPort;
let seen=0,allRequests=[];
function clearLog(){allRequests=[];seen=0;render();fetch('/api/requests/clear',{method:'POST'}).catch(()=>{});}
function statusClass(s){if(!s)return '';if(s>=500)return 's-err';if(s>=400)return 's-err';if(s>=300)return 's-redir';if(s>=200)return 's-ok';return 's-info';}
function durClass(ms){return ms>800?'slow':'ok';}
function timeStr(ts){return new Date(ts).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit',second:'2-digit'});}
function pathOf(url){try{return new URL(url).pathname||'/';}catch{return url||'/';}}
function render(){
  const app=document.getElementById('app');
  const n=allRequests.length;
  document.getElementById('nav-count').textContent=n+' request'+(n!==1?'s':'');
  if(n===0){app.innerHTML='<div class="empty"><div>Waiting for requests…</div><div class="hint">Make a request to your tunnel URL to see it here.</div></div>';return;}
  const rows=[...allRequests].reverse().map((r,i)=>{
    const m=r.method||'GET',s=r.status_code||0,ms=r.duration_ms||0;
    return '<tr class="'+(i===0&&seen<n?'new-row':'')+'">'+
      '<td><span class="badge '+m+'">'+m+'</span></td>'+
      '<td class="path">'+pathOf(r.url)+'</td>'+
      '<td class="'+statusClass(s)+'">'+(s||'—')+'</td>'+
      '<td class="dur '+durClass(ms)+'">'+ms+'ms</td>'+
      '<td class="ts">'+timeStr(r.timestamp)+'</td></tr>';
  }).join('');
  app.innerHTML='<div class="tw"><table><thead><tr><th style="width:72px">Method</th><th>Path</th><th style="width:72px">Status</th><th style="width:80px">Duration</th><th style="width:90px">Time</th></tr></thead><tbody>'+rows+'</tbody></table></div>';
}
async function poll(){
  try{const res=await fetch('/api/requests');if(!res.ok)return;const data=await res.json();const reqs=data.requests||[];
  if(reqs.length!==allRequests.length){const prev=allRequests.length;allRequests=reqs;seen=prev;render();}}catch(_){}
}
setInterval(poll,800);poll();
</script>
</body>
</html>`
