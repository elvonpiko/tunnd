// Tests for the pure server-side request-preparation helpers in forward.go.
//
// These tests live in `package tunnel` (not `tunnel_test`) so they can call
// the unexported helpers `applyHostHeader` and `setForwardedHeaders` directly
// without going through the full Registry / WebSocket plumbing.
package tunnel

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/quick"
)

// ── applyHostHeader ──────────────────────────────────────────────────────────

// TestApplyHostHeader_Rewrite verifies the default "rewrite" policy replaces
// both req.Host AND req.Header["Host"] with "localhost:<LocalPort>".
//
// On unfixed code (where applyHostHeader does not exist or is not wired in),
// req.Host would be left as "myapp.tunnd.example" and the test would fail.
func TestApplyHostHeader_Rewrite(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.Header.Set("Host", "myapp.tunnd.example")

	sess := &Session{HostHeader: "rewrite", LocalPort: 3000}
	applyHostHeader(req, sess)

	if got, want := req.Host, "localhost:3000"; got != want {
		t.Errorf("req.Host = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Host"), "localhost:3000"; got != want {
		t.Errorf("req.Header[Host] = %q, want %q", got, want)
	}
}

// TestApplyHostHeader_RewriteEmptyDefault verifies that an empty HostHeader
// string is treated as the default "rewrite" policy. The new default makes
// Vite / Next / webpack-dev-server work out of the box.
func TestApplyHostHeader_RewriteEmptyDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.Header.Set("Host", "myapp.tunnd.example")

	sess := &Session{HostHeader: "", LocalPort: 3000}
	applyHostHeader(req, sess)

	if got, want := req.Host, "localhost:3000"; got != want {
		t.Errorf("req.Host = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Host"), "localhost:3000"; got != want {
		t.Errorf("req.Header[Host] = %q, want %q", got, want)
	}
}

// TestApplyHostHeader_Preserve verifies the "preserve" policy is a no-op:
// the public Host is forwarded verbatim (the explicit opt-out path for
// users whose upstream depends on the original Host).
func TestApplyHostHeader_Preserve(t *testing.T) {
	const original = "myapp.tunnd.example"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = original
	req.Header.Set("Host", original)

	sess := &Session{HostHeader: "preserve", LocalPort: 3000}
	applyHostHeader(req, sess)

	if got := req.Host; got != original {
		t.Errorf("req.Host = %q, want %q (unchanged)", got, original)
	}
	if got := req.Header.Get("Host"); got != original {
		t.Errorf("req.Header[Host] = %q, want %q (unchanged)", got, original)
	}
}

// TestApplyHostHeader_Literal verifies that any value other than "",
// "rewrite", or "preserve" is treated as a literal hostname and applied
// verbatim to both req.Host and req.Header["Host"].
func TestApplyHostHeader_Literal(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.Header.Set("Host", "myapp.tunnd.example")

	sess := &Session{HostHeader: "myapp.local", LocalPort: 3000}
	applyHostHeader(req, sess)

	if got, want := req.Host, "myapp.local"; got != want {
		t.Errorf("req.Host = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Host"), "myapp.local"; got != want {
		t.Errorf("req.Header[Host] = %q, want %q", got, want)
	}
}

// TestApplyHostHeader_RewriteZeroPortNoOp verifies the defensive guard:
// a rewrite policy with LocalPort == 0 is a no-op rather than producing
// "localhost:0", which would confuse upstreams. (TCP tunnels don't go
// through this code path, but defending against the zero-value avoids
// surprising behavior if the field is ever unset.)
func TestApplyHostHeader_RewriteZeroPortNoOp(t *testing.T) {
	const original = "myapp.tunnd.example"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = original
	req.Header.Set("Host", original)

	sess := &Session{HostHeader: "rewrite", LocalPort: 0}
	applyHostHeader(req, sess)

	if got := req.Host; got != original {
		t.Errorf("req.Host = %q, want %q (unchanged on LocalPort=0)", got, original)
	}
	if got := req.Header.Get("Host"); got != original {
		t.Errorf("req.Header[Host] = %q, want %q (unchanged on LocalPort=0)", got, original)
	}
}

// TestApplyHostHeader_NilSafe verifies the helper is a no-op (and does
// not panic) when either req or sess is nil. Defensive programming for
// edge paths that might not have a session bound yet.
func TestApplyHostHeader_NilSafe(t *testing.T) {
	// Nil session must not panic.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	applyHostHeader(req, nil)
	if got, want := req.Host, "myapp.tunnd.example"; got != want {
		t.Errorf("nil sess: req.Host = %q, want %q (unchanged)", got, want)
	}

	// Nil request must not panic either.
	sess := &Session{HostHeader: "rewrite", LocalPort: 3000}
	applyHostHeader(nil, sess) // must not panic
}

// ── setForwardedHeaders ──────────────────────────────────────────────────────

// TestSetForwardedHeaders_Append verifies that an existing X-Forwarded-For
// chain is preserved and the new client IP is appended per RFC 7239
// conventions ("a, b" comma-space separated).
func TestSetForwardedHeaders_Append(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-For"), "10.0.0.1, 203.0.113.7"; got != want {
		t.Errorf("X-Forwarded-For = %q, want %q", got, want)
	}
}

// TestSetForwardedHeaders_New verifies that with no pre-existing
// X-Forwarded-For chain, the helper sets the header to just the client IP
// (no leading comma, no whitespace).
func TestSetForwardedHeaders_New(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-For"), "203.0.113.7"; got != want {
		t.Errorf("X-Forwarded-For = %q, want %q", got, want)
	}
}

// TestSetForwardedHeaders_Proto_FromCaddy verifies that an inbound
// X-Forwarded-Proto (typically set by Caddy in production) is preserved
// verbatim and NOT overwritten by the helper.
func TestSetForwardedHeaders_Proto_FromCaddy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"
	req.Header.Set("X-Forwarded-Proto", "https")

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-Proto"), "https"; got != want {
		t.Errorf("X-Forwarded-Proto = %q, want %q (preserved)", got, want)
	}
}

// TestSetForwardedHeaders_Proto_DefaultProduction verifies that with no
// inbound X-Forwarded-Proto, the helper defaults to "https" — the
// production assumption since Caddy fronts the tunnel server with TLS.
func TestSetForwardedHeaders_Proto_DefaultProduction(t *testing.T) {
	// Ensure dev-mode env var does not leak into this test.
	t.Setenv("TUNND_DEV_MODE", "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-Proto"), "https"; got != want {
		t.Errorf("X-Forwarded-Proto = %q, want %q (production default)", got, want)
	}
}

// TestSetForwardedHeaders_Proto_DevMode verifies that with TUNND_DEV_MODE=1
// and no inbound X-Forwarded-Proto, the helper defaults to "http" — the
// local-dev assumption when no TLS-terminating proxy fronts the server.
func TestSetForwardedHeaders_Proto_DevMode(t *testing.T) {
	t.Setenv("TUNND_DEV_MODE", "1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-Proto"), "http"; got != want {
		t.Errorf("X-Forwarded-Proto = %q, want %q (dev-mode default)", got, want)
	}
}

// TestSetForwardedHeaders_Host verifies that X-Forwarded-Host is set to the
// originalHost passed in (the public Host captured BEFORE applyHostHeader
// rewrote req.Host).
func TestSetForwardedHeaders_Host(t *testing.T) {
	const originalHost = "myapp.tunnd.example"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Simulate post-rewrite state: req.Host has already been rewritten to
	// localhost:<port>, but the originalHost is still passed in.
	req.Host = "localhost:3000"
	req.RemoteAddr = "203.0.113.7:50000"

	setForwardedHeaders(req, originalHost)

	if got, want := req.Header.Get("X-Forwarded-Host"), originalHost; got != want {
		t.Errorf("X-Forwarded-Host = %q, want %q", got, want)
	}
}

// TestSetForwardedHeaders_HostFallback verifies that when originalHost is
// empty, X-Forwarded-Host falls back to req.Host so the header is never
// blank.
func TestSetForwardedHeaders_HostFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "203.0.113.7:50000"

	setForwardedHeaders(req, "")

	if got, want := req.Header.Get("X-Forwarded-Host"), "myapp.tunnd.example"; got != want {
		t.Errorf("X-Forwarded-Host = %q, want %q (fallback to req.Host)", got, want)
	}
}

// TestSetForwardedHeaders_NoPortInRemoteAddr verifies that a RemoteAddr
// without a port (uncommon, but possible for unix sockets or test fixtures)
// is treated as a bare IP and produces the correct X-Forwarded-For value.
func TestSetForwardedHeaders_NoPortInRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "10.0.0.5"

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-For"), "10.0.0.5"; got != want {
		t.Errorf("X-Forwarded-For = %q, want %q (bare IP, no port)", got, want)
	}
}

// TestSetForwardedHeaders_IPv6RemoteAddr verifies that an IPv6 RemoteAddr
// in the canonical "[::1]:50000" form parses cleanly to "::1" (without
// the surrounding brackets) for X-Forwarded-For.
func TestSetForwardedHeaders_IPv6RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myapp.tunnd.example"
	req.RemoteAddr = "[::1]:50000"

	setForwardedHeaders(req, "myapp.tunnd.example")

	if got, want := req.Header.Get("X-Forwarded-For"), "::1"; got != want {
		t.Errorf("X-Forwarded-For = %q, want %q (IPv6 literal)", got, want)
	}
}

// ── Property-based tests ─────────────────────────────────────────────────────

// xffInputs carries the random inputs for the X-Forwarded-For property.
//
// All fields MUST be exported so testing/quick can populate them via
// reflection.
//
// The fields are:
//   - ChainLen: 0..3 random pre-existing entries (len modulo 4)
//   - Chain:    12 random bytes interpreted as 3 IPv4 addresses; the
//     first ChainLen of them form the pre-existing chain
//   - ClientIP: 4 random bytes — the new client IP appended on this call
//   - EmptyIP:  if true, the helper is invoked with an empty RemoteAddr
//     (the helper SHALL leave X-Forwarded-For untouched in that
//     case; the property simply returns true and skips the check)
type xffInputs struct {
	ChainLen byte
	Chain    [12]byte
	ClientIP [4]byte
	EmptyIP  bool
}

// formatIPv4 renders four bytes as a dotted-quad IPv4 string.
func formatIPv4(b [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

// TestProperty_XForwardedForAppend verifies the X-Forwarded-For chain
// produced by setForwardedHeaders is well-formed RFC 7239 for arbitrary
// pre-existing chains and arbitrary client IPs:
//
//   - comma-separated with single-space delimiters (", ")
//   - no leading or trailing whitespace
//   - no empty entries
//   - ends with the new client IP
//   - preserves the order of the pre-existing entries
//
// Generators that occasionally produce an empty client IP (emptyIP=true)
// are fine; the property simply returns true for those samples since the
// helper is documented to skip XFF entirely when no client IP is present.
//
// Validates: P3 (X-Forwarded-For Append — RFC 7239 chain correctness).
func TestProperty_XForwardedForAppend(t *testing.T) {
	property := func(in xffInputs) bool {
		// Build the pre-existing chain from ChainLen random IPv4 entries.
		nPre := int(in.ChainLen) % 4 // 0..3
		preEntries := make([]string, 0, nPre)
		for i := 0; i < nPre; i++ {
			var ip [4]byte
			copy(ip[:], in.Chain[i*4:(i+1)*4])
			preEntries = append(preEntries, formatIPv4(ip))
		}
		preChain := strings.Join(preEntries, ", ")

		// Build the request. EmptyIP samples invoke the helper with no
		// RemoteAddr at all — the helper does the right thing (skips XFF)
		// and we return true to skip the check.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "myapp.tunnd.example"
		if in.EmptyIP {
			req.RemoteAddr = ""
		} else {
			req.RemoteAddr = formatIPv4(in.ClientIP) + ":50000"
		}
		if preChain != "" {
			req.Header.Set("X-Forwarded-For", preChain)
		}

		setForwardedHeaders(req, "myapp.tunnd.example")

		// Empty client IP: helper SHALL not modify XFF; nothing to check.
		if in.EmptyIP {
			got := req.Header.Get("X-Forwarded-For")
			return got == preChain
		}

		got := req.Header.Get("X-Forwarded-For")
		newIP := formatIPv4(in.ClientIP)

		// No leading or trailing whitespace.
		if got != strings.TrimSpace(got) {
			t.Logf("leading/trailing whitespace: %q", got)
			return false
		}

		// Split on the canonical RFC 7239 ", " delimiter and verify
		// every entry is well-formed.
		parts := strings.Split(got, ", ")
		for _, p := range parts {
			if p == "" {
				t.Logf("empty entry in chain: %q", got)
				return false
			}
			if p != strings.TrimSpace(p) {
				t.Logf("entry has surrounding whitespace: %q (in %q)", p, got)
				return false
			}
		}

		// The last entry must be the newly appended client IP.
		if parts[len(parts)-1] != newIP {
			t.Logf("last entry %q != client IP %q (chain %q)",
				parts[len(parts)-1], newIP, got)
			return false
		}

		// The number of parts must equal preEntries + 1 — anything else
		// means a stray comma split the chain or an entry was dropped.
		if len(parts) != len(preEntries)+1 {
			t.Logf("part count %d != expected %d (chain %q)",
				len(parts), len(preEntries)+1, got)
			return false
		}

		// Pre-existing entries must appear in the original order.
		for i, want := range preEntries {
			if parts[i] != want {
				t.Logf("entry %d = %q, want %q (chain %q)",
					i, parts[i], want, got)
				return false
			}
		}

		return true
	}

	if err := quick.Check(property, nil); err != nil {
		t.Errorf("X-Forwarded-For append property failed: %v", err)
	}
}

// hostHeaderInputs carries the random inputs for the host-header
// idempotence property.
//
// All fields MUST be exported so testing/quick can populate them via
// reflection. The fields are decoded into "real" inputs as follows:
//
//   - HostBytes / HostLen: a non-empty random alphanumeric+dot+colon string
//     used as the public req.Host. Length = (HostLen % 16) + 1.
//   - PolicyIdx: enum-style selector, modulo 4, exercises every branch:
//     0 → "" (default-rewrite), 1 → "rewrite", 2 → "preserve",
//     3 → a random literal hostname.
//   - LiteralBytes / LiteralLen: only used when PolicyIdx % 4 == 3; same
//     alphanumeric+dot+colon mapping as HostBytes.
//   - LocalPort: uint16 — covers the full HTTP port range.
//   - ZeroPort: when true, force LocalPort = 0 to exercise the
//     defensive no-op branch (rewrite + LocalPort==0 leaves req.Host
//     unchanged; idempotence on a no-op is trivially true).
type hostHeaderInputs struct {
	HostBytes    [16]byte
	HostLen      byte
	PolicyIdx    byte
	LiteralBytes [16]byte
	LiteralLen   byte
	LocalPort    uint16
	ZeroPort     bool
}

// hostCharset is the alphanumeric+dot+colon alphabet used to map random
// bytes into strings that httptest.NewRequest / req.Host accept without
// fuss. It deliberately excludes characters that would make the Host
// header malformed (whitespace, control characters, etc.).
const hostCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.:"

// mapHostBytes projects a slice of random bytes onto hostCharset, producing
// a string of the same length composed only of safe Host characters.
func mapHostBytes(b []byte) string {
	s := make([]byte, len(b))
	for i, x := range b {
		s[i] = hostCharset[int(x)%len(hostCharset)]
	}
	return string(s)
}

// TestProperty_HostHeaderApplyIdempotent verifies that applyHostHeader is
// idempotent: calling it twice produces the same final state as calling it
// once. The property covers every policy branch ("", "rewrite", "preserve",
// arbitrary literal) AND the defensive LocalPort==0 no-op path.
//
// Concretely: for any random req.Host, any of the four policy values, and
// any LocalPort (including 0), after
//
//	applyHostHeader(req, sess)
//	applyHostHeader(req, sess)
//
// both req.Host and req.Header.Get("Host") equal the values they had after
// the first call.
//
// Validates: P2 (Host-Header Apply Idempotence).
func TestProperty_HostHeaderApplyIdempotent(t *testing.T) {
	property := func(in hostHeaderInputs) bool {
		// Random non-empty alphanumeric+dot+colon public Host.
		hostLen := (int(in.HostLen) % 16) + 1
		publicHost := mapHostBytes(in.HostBytes[:hostLen])

		// Enum-style policy selector — exercises all four branches with
		// roughly equal probability.
		var policy string
		switch in.PolicyIdx % 4 {
		case 0:
			policy = "" // default-rewrite branch
		case 1:
			policy = "rewrite"
		case 2:
			policy = "preserve"
		default:
			litLen := (int(in.LiteralLen) % 16) + 1
			policy = mapHostBytes(in.LiteralBytes[:litLen])
		}

		// Random LocalPort, occasionally forced to 0 to exercise the
		// defensive no-op branch in applyHostHeader.
		port := int(in.LocalPort)
		if in.ZeroPort {
			port = 0
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = publicHost
		req.Header.Set("Host", publicHost)

		sess := &Session{HostHeader: policy, LocalPort: port}

		// First application — capture the post-call state.
		applyHostHeader(req, sess)
		afterFirstHost := req.Host
		afterFirstHeader := req.Header.Get("Host")

		// Second application — must not change anything.
		applyHostHeader(req, sess)

		if req.Host != afterFirstHost {
			t.Logf("req.Host diverged: after-first=%q after-second=%q "+
				"(policy=%q, port=%d, publicHost=%q)",
				afterFirstHost, req.Host, policy, port, publicHost)
			return false
		}
		if got := req.Header.Get("Host"); got != afterFirstHeader {
			t.Logf("req.Header[Host] diverged: after-first=%q after-second=%q "+
				"(policy=%q, port=%d, publicHost=%q)",
				afterFirstHeader, got, policy, port, publicHost)
			return false
		}

		return true
	}

	if err := quick.Check(property, nil); err != nil {
		t.Errorf("Host-Header apply idempotence property failed: %v", err)
	}
}
