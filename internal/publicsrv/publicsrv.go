// Package publicsrv assembles the tunnd-server public listener: the request
// mux (WebSocket control plane + Host-based routing) and the http.Server
// carrying the production timeout posture.
//
// It lives in internal/ so the integration suite can build the exact same
// listener the real server runs and verify long-streaming behavior against
// it — the previous suite tested against a timeout-less httptest server,
// so the production WriteTimeout (which killed SSE at 90s) was invisible
// to CI.
package publicsrv

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"
)

// ControlPath is the WebSocket control-plane endpoint on the public mux.
const ControlPath = "/_tunnd/control"

// NewMux builds the public request mux exactly as tunnd-server serves it:
// the WebSocket control plane at ControlPath, and Host-based routing for
// everything else — the bare base domain goes to the admin dashboard so
// operators can reach it over HTTPS at https://<domain>, while subdomain
// traffic goes to the tunnel registry.
func NewMux(domain string, control, registry, admin http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(ControlPath, control)
	mux.Handle("/", rootHandler(domain, registry, admin))
	return mux
}

// NewServer returns the public http.Server with the production timeout
// posture:
//
//   - ReadHeaderTimeout (30s) bounds slowloris-style header reads.
//   - ReadTimeout is intentionally UNSET: it would cap tunneled request
//     bodies (uploads) at a fixed wall clock, which is wrong for a proxy.
//   - WriteTimeout is intentionally UNSET: tunneled responses (SSE, long
//     polls, large downloads) may legitimately stream for minutes. The old
//     90s WriteTimeout killed them mid-flight. Liveness is enforced by the
//     tunnel layer instead: a dead client tears down streams via the
//     control plane (pong deadline), unblocking the response reader.
//   - IdleTimeout (120s) bounds keep-alive gaps between requests.
//
// Callers own the TLSConfig: pass nil for plain-HTTP (dev / behind a
// reverse proxy).
func NewServer(addr string, handler http.Handler, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// rootHandler dispatches public requests by Host. Requests addressed to the
// bare base domain (e.g. https://tunnd.example.com) are served by the admin
// dashboard; everything else (subdomain tunnel traffic) goes to the registry.
// This lets operators reach the dashboard over HTTPS on the public domain
// while the admin port stays available for reverse-proxy / LAN access.
func rootHandler(domain string, registry, admin http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i != -1 {
			host = host[:i]
		}
		if host == domain {
			admin.ServeHTTP(w, r)
			return
		}
		registry.ServeHTTP(w, r)
	})
}
