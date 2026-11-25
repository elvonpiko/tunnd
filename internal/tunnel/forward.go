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
// Tunnd is often the edge, so a public peer does not get to choose these
// headers: X-Forwarded-For is replaced with that peer, and X-Forwarded-Proto
// comes from the connection tunnd accepted (https when req.TLS is set).
//
// A loopback, link-local, or private peer is treated as a reverse proxy on
// the same machine (Caddy in Docker, for example). In that case an inbound
// X-Forwarded-For chain is kept and the peer is appended, and an inbound
// X-Forwarded-Proto is kept.
//
// X-Forwarded-Host is always the original public Host captured before
// applyHostHeader rewrites req.Host. An empty originalHost falls back to
// req.Host so the header is never blank.
func setForwardedHeaders(req *http.Request, originalHost string) {
	if req == nil {
		return
	}

	clientIP := clientIPFromRemoteAddr(req.RemoteAddr)
	if clientIP != "" {
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		if peerIsTrustedProxy(clientIP) {
			if existing := req.Header.Get("X-Forwarded-For"); existing != "" {
				req.Header.Set("X-Forwarded-For", existing+", "+clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", clientIP)
			}
			if req.Header.Get("X-Forwarded-Proto") == "" {
				req.Header.Set("X-Forwarded-Proto", proto)
			}
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
			req.Header.Set("X-Forwarded-Proto", proto)
		}
	}

	host := originalHost
	if host == "" {
		host = req.Host
	}
	if host != "" {
		req.Header.Set("X-Forwarded-Host", host)
	}
}

// peerIsTrustedProxy reports whether ip is a loopback, link-local, or
// private address — the peers that sit in front of tunnd when a local
// reverse proxy terminates TLS.
func peerIsTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast()
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
