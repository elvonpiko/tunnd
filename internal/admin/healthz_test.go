package admin_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestHealthz_Public confirms /healthz is reachable without auth, returns
// 200 OK with a plain-text body, and works in both bootstrap and configured
// modes. Docker HEALTHCHECK and external probes depend on this contract.
func TestHealthz_Public(t *testing.T) {
	for _, password := range []string{"secret", ""} {
		password := password
		name := "configured"
		if password == "" {
			name = "bootstrap"
		}
		t.Run(name, func(t *testing.T) {
			h, _ := setup(t, password)
			w := req(t, h, "GET", "/healthz", nil, "")
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}
			if body := strings.TrimSpace(w.Body.String()); body != "ok" {
				t.Errorf("body = %q, want %q", body, "ok")
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
				t.Errorf("content-type = %q, want text/plain*", ct)
			}
		})
	}
}
