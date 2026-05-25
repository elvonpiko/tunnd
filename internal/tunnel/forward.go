// Server-side request preparation helpers used by both Registry.ServeHTTP
// and handleUpgrade.
//
// These helpers are pure functions: they only read from req / sess / the
// process environment and write to req. No I/O, no goroutines, no logging —
// so they can be property-tested cleanly.
package tunnel

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// applyHostHeader rewrites req.Host and req.Header["Host"] according to the
// session's host-header policy. Both req.Host AND the explicit "Host" header
// are set because Go's req.Write serialises req.Host into the request line,
// and downstream code may also read the Header["Host"] entry.
//
// Policy:
//   - "preserve": no-op (forward the public Host unchanged)
//   - "" or "rewrite": replace with "localhost:<sess.LocalPort>" (the new
//     default; makes Vite / Next / webpack-dev-server work out of the box)
//   - any other value: treat as a literal hostname and use it verbatim
//
// Defensive behavior: if sess is nil, this is a no-op. In the rewrite case
// when sess.LocalPort is 0 (unusual — TCP tunnels don't go through this
// code path), req.Host is left unchanged rather than producing a
// "localhost:0" header that would confuse upstream servers.
func applyHostHeader(req *http.Request, sess *Session) {
	if req == nil || sess == nil {
		return
	}

	switch sess.HostHeader {
	case "preserve":
		// Forward the public Host header verbatim — opt-out path for
		// users whose upstream depends on the original Host.
		return

	case "", "rewrite":
		// Default: rewrite to localhost:<port> so default-configured
		// dev servers accept the request.
		if sess.LocalPort <= 0 {
			// LocalPort 0 is unusual; leave req.Host alone to avoid
			// emitting "localhost:0".
			return
		}
		host := "localhost:" + strconv.Itoa(sess.LocalPort)
		req.Host = host
		req.Header.Set("Host", host)

	default:
		// Literal hostname (e.g. "myapp.local", "localhost:3000").
		req.Host = sess.HostHeader
		req.Header.Set("Host", sess.HostHeader)
	}
}

// setForwardedHeaders injects the standard X-Forwarded-* trio onto req
// before it is serialised into the stream pipe.
//
//   - X-Forwarded-For: append the client IP (parsed from req.RemoteAddr)
//     to any pre-existing chain per RFC 7239 conventions.
//   - X-Forwarded-Proto: preserve any inbound value (Caddy may have set it).
//     Otherwise default to "https" — production assumption, since Caddy
//     fronts the tunnel server with TLS. The env var TUNND_DEV_MODE=1
//     flips the default to "http" for local dev runs without a TLS front.
//   - X-Forwarded-Host: set to originalHost (the public Host captured
//     BEFORE applyHostHeader rewrites req.Host). If originalHost is empty,
//     fall back to req.Host so the header is never blank.
//
// Reading TUNND_DEV_MODE is the only side-effect; it's idempotent and
// process-scoped so it doesn't compromise the helper's testability.
func setForwardedHeaders(req *http.Request, originalHost string) {
	if req == nil {
		return
	}

	// X-Forwarded-For: parse client IP, then append to any existing chain.
	clientIP := clientIPFromRemoteAddr(req.RemoteAddr)
	if clientIP != "" {
		if existing := req.Header.Get("X-Forwarded-For"); existing != "" {
			req.Header.Set("X-Forwarded-For", existing+", "+clientIP)
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
	}

	// X-Forwarded-Proto: preserve inbound (Caddy), else default by env.
	if req.Header.Get("X-Forwarded-Proto") == "" {
		proto := "https"
		if os.Getenv("TUNND_DEV_MODE") == "1" {
			proto = "http"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
	}

	// X-Forwarded-Host: original public Host, falling back to req.Host
	// so the header is always populated.
	host := originalHost
	if host == "" {
		host = req.Host
	}
	if host != "" {
		req.Header.Set("X-Forwarded-Host", host)
	}
}

// clientIPFromRemoteAddr extracts the IP portion of a "host:port" remote
// address. If the address has no port (uncommon, but possible for unix
// sockets or test fixtures), the whole string is treated as the IP.
// Returns "" only if the input is empty.
func clientIPFromRemoteAddr(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// No port — use the address verbatim. Trim brackets that may
		// wrap a bare IPv6 literal (e.g. "[::1]").
		return strings.Trim(remoteAddr, "[]")
	}
	return host
}
